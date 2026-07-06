// Package main is a Drone / Woodpecker plugin that syncs content to Gitea wiki pages.
//
// Gitea wiki is a git repo (<repo>.wiki.git). This plugin clones it, writes a
// markdown file, commits, and pushes — a simple git-backed wiki publisher.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"codeberg.org/woodpecker-plugins/go-plugin"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

// Settings holds all plugin configuration from workflow settings.
type Settings struct {
	GiteaURL   string // Gitea instance URL (e.g., https://gitea.example.com)
	GiteaToken string // API token for git auth
	WikiRepo   string // target repo: "owner/repo"
	Page       string // wiki page path, e.g. "code-review/2024-07-06.md"
	Content    string // markdown content to write
	Message    string // commit message (default: auto-generated)
	GitUser    string // commit author name (default: plugin-gitea-wiki)
	GitEmail   string // commit author email (default: plugin@ci)
}

func defaults() *Settings {
	return &Settings{
		GitUser:  "plugin-gitea-wiki",
		GitEmail: "plugin@ci",
	}
}

// Plugin wraps the base plugin with parsed settings.
type Plugin struct {
	*plugin.Plugin
	Settings *Settings
}

// Flags maps PLUGIN_* env vars to Settings fields.
func (p *Plugin) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "gitea-url",
			Usage:       "Gitea instance URL (e.g., https://gitea.example.com)",
			Sources:     cli.EnvVars("PLUGIN_GITEA_URL", "GITEA_URL"),
			Destination: &p.Settings.GiteaURL,
		},
		&cli.StringFlag{
			Name:        "gitea-token",
			Usage:       "Gitea API token for git auth",
			Sources:     cli.EnvVars("PLUGIN_GITEA_TOKEN"),
			Destination: &p.Settings.GiteaToken,
		},
		&cli.StringFlag{
			Name:        "wiki-repo",
			Usage:       "target repository (owner/repo)",
			Sources:     cli.EnvVars("PLUGIN_WIKI_REPO", "CI_REPO"),
			Destination: &p.Settings.WikiRepo,
		},
		&cli.StringFlag{
			Name:        "page",
			Usage:       "wiki page path, e.g. code-review/2024-07-06.md",
			Sources:     cli.EnvVars("PLUGIN_PAGE"),
			Destination: &p.Settings.Page,
		},
		&cli.StringFlag{
			Name:        "content",
			Usage:       "markdown content to write to the wiki page",
			Sources:     cli.EnvVars("PLUGIN_CONTENT"),
			Destination: &p.Settings.Content,
		},
		&cli.StringFlag{
			Name:        "message",
			Usage:       "commit message (default: auto-generated)",
			Sources:     cli.EnvVars("PLUGIN_MESSAGE"),
			Destination: &p.Settings.Message,
		},
		&cli.StringFlag{
			Name:        "git-user",
			Usage:       "commit author name",
			Value:       p.Settings.GitUser,
			Sources:     cli.EnvVars("PLUGIN_GIT_USER"),
			Destination: &p.Settings.GitUser,
		},
		&cli.StringFlag{
			Name:        "git-email",
			Usage:       "commit author email",
			Value:       p.Settings.GitEmail,
			Sources:     cli.EnvVars("PLUGIN_GIT_EMAIL"),
			Destination: &p.Settings.GitEmail,
		},
	}
}

// Execute clones the wiki repo, writes the page, commits, and pushes.
func (p *Plugin) Execute(ctx context.Context) error {
	s := p.Settings

	if err := s.validate(); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	if s.Message == "" {
		s.Message = fmt.Sprintf("Update wiki page %s", s.Page)
	}

	workDir := "/tmp/wiki"
	defer os.RemoveAll(workDir) //nolint:errcheck

	// Clone the wiki repo
	wikiURL := fmt.Sprintf("%s/%s.wiki.git", s.GiteaURL, s.WikiRepo)
	log.Info().Str("url", wikiURL).Msg("cloning wiki repo")

	if err := run(ctx, workDir, "git", "clone", "--depth", "1", wikiURL, "."); err != nil {
		// Try without --depth 1 (older git or empty wiki)
		log.Warn().Msg("shallow clone failed, retrying with full clone")
		if err := run(ctx, workDir, "git", "clone", wikiURL, "."); err != nil {
			return fmt.Errorf("clone wiki repo: %w", err)
		}
	}

	// Write the page
	pagePath := filepath.Join(workDir, s.Page)
	if err := os.MkdirAll(filepath.Dir(pagePath), 0755); err != nil {
		return fmt.Errorf("create page directory: %w", err)
	}
	if err := os.WriteFile(pagePath, []byte(s.Content), 0644); err != nil {
		return fmt.Errorf("write page %s: %w", s.Page, err)
	}

	// Configure git with token-based auth
	authURL := fmt.Sprintf("%s/%s.wiki.git", s.GiteaURL, s.WikiRepo)
	escapedToken := strings.ReplaceAll(s.GiteaToken, "@", "%40")
	escapedToken = strings.ReplaceAll(escapedToken, ":", "%3A")
	pushURL := fmt.Sprintf("https://plugin:%s@%s",
		escapedToken,
		strings.TrimPrefix(authURL, "https://"))

	if err := run(ctx, workDir, "git", "config", "user.name", s.GitUser); err != nil {
		return fmt.Errorf("set git user: %w", err)
	}
	if err := run(ctx, workDir, "git", "config", "user.email", s.GitEmail); err != nil {
		return fmt.Errorf("set git email: %w", err)
	}

	// Set the remote URL with token for push
	if err := run(ctx, workDir, "git", "remote", "set-url", "origin", pushURL); err != nil {
		return fmt.Errorf("set remote url: %w", err)
	}

	// Stage, commit, push
	if err := run(ctx, workDir, "git", "add", s.Page); err != nil {
		return fmt.Errorf("stage page: %w", err)
	}
	if err := run(ctx, workDir, "git", "commit", "-m", s.Message); err != nil {
		log.Warn().Msg("commit (may be empty — page unchanged)")
		// Don't fail if nothing to commit
		return nil
	}
	if err := run(ctx, workDir, "git", "push"); err != nil {
		return fmt.Errorf("push: %w", err)
	}

	log.Info().Str("page", s.Page).Msg("wiki page synced")
	return nil
}

func (s *Settings) validate() error {
	if s.GiteaURL == "" {
		return fmt.Errorf("gitea_url is required")
	}
	if s.GiteaToken == "" {
		return fmt.Errorf("gitea_token is required")
	}
	if s.WikiRepo == "" {
		return fmt.Errorf("wiki_repo is required")
	}
	if s.Page == "" {
		return fmt.Errorf("page is required")
	}
	if s.Content == "" {
		return fmt.Errorf("content is required (markdown to write)")
	}
	return nil
}

func run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=echo",
	)
	return cmd.Run()
}

func main() {
	s := defaults()
	p := &Plugin{Settings: s}
	p.Plugin = plugin.New(plugin.Options{
		Name:        "plugin-gitea-wiki",
		Description: "Sync content to Gitea wiki pages",
		Version:     "0.1.0",
		Flags:       p.Flags(),
		Execute:     p.Execute,
	})
	p.Run()
}
