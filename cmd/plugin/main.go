// Package main is a Drone / Woodpecker plugin that writes pages to Gitea wiki
// via the REST API (POST /repos/{owner}/{repo}/wiki/new).
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"codeberg.org/woodpecker-plugins/go-plugin"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

// Settings holds all plugin configuration from workflow settings.
type Settings struct {
	GiteaURL    string // https://gitea.example.com
	GiteaToken  string // API token
	Repo        string // owner/repo
	Page        string // wiki page title, e.g. "code-review/2024-07-06"
	Content     string // markdown content (inline)
	ContentFile string // path to file containing markdown content
}

func defaults() *Settings {
	return &Settings{}
}

type Plugin struct {
	*plugin.Plugin
	Settings *Settings
}

func (p *Plugin) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "gitea-url",
			Usage:       "Gitea instance URL (default: CI_SYSTEM_URL)",
			Sources:     cli.EnvVars("PLUGIN_GITEA_URL", "CI_SYSTEM_URL"),
			Destination: &p.Settings.GiteaURL,
		},
		&cli.StringFlag{
			Name:        "gitea-token",
			Usage:       "Gitea API token (default: CI_JOB_TOKEN)",
			Sources:     cli.EnvVars("PLUGIN_GITEA_TOKEN", "CI_JOB_TOKEN"),
			Destination: &p.Settings.GiteaToken,
		},
		&cli.StringFlag{
			Name:        "repo",
			Usage:       "target repository (owner/repo)",
			Sources:     cli.EnvVars("PLUGIN_REPO", "CI_REPO"),
			Destination: &p.Settings.Repo,
		},
		&cli.StringFlag{
			Name:        "page",
			Usage:       "wiki page title, e.g. Code-Review/2024-07-06",
			Sources:     cli.EnvVars("PLUGIN_PAGE"),
			Destination: &p.Settings.Page,
		},
		&cli.StringFlag{
			Name:        "content",
			Usage:       "markdown content (inline; use content_file for large content)",
			Sources:     cli.EnvVars("PLUGIN_CONTENT"),
			Destination: &p.Settings.Content,
		},
		&cli.StringFlag{
			Name:        "content-file",
			Usage:       "path to file containing markdown content",
			Sources:     cli.EnvVars("PLUGIN_CONTENT_FILE"),
			Destination: &p.Settings.ContentFile,
		},
	}
}

func (p *Plugin) Execute(ctx context.Context) error {
	s := p.Settings
	if err := s.validate(); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// Resolve content: file takes priority over inline
	if s.ContentFile != "" {
		data, err := os.ReadFile(s.ContentFile)
		if err != nil {
			return fmt.Errorf("read content_file %s: %w", s.ContentFile, err)
		}
		s.Content = string(data)
	}

	// Gitea wiki API: content must be base64-encoded
	body := map[string]string{
		"title":   s.Page,
		"content": base64.StdEncoding.EncodeToString([]byte(s.Content)),
	}
	payload, _ := json.Marshal(body)

	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/wiki/new",
		strings.TrimRight(s.GiteaURL, "/"), s.Repo)

	log.Info().Str("url", apiURL).Str("page", s.Page).Msg("creating wiki page")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+s.GiteaToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		log.Info().Str("page", s.Page).Msg("wiki page created")
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("wiki API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}

func (s *Settings) validate() error {
	if s.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if s.Page == "" {
		return fmt.Errorf("page is required")
	}
	if s.Content == "" && s.ContentFile == "" {
		return fmt.Errorf("content or content_file is required")
	}
	return nil
}

func main() {
	s := defaults()
	p := &Plugin{Settings: s}
	p.Plugin = plugin.New(plugin.Options{
		Name:        "plugin-gitea-wiki",
		Description: "Publish pages to Gitea wiki via REST API",
		Version:     "0.1.0",
		Flags:       p.Flags(),
		Execute:     p.Execute,
	})
	p.Run()
}
