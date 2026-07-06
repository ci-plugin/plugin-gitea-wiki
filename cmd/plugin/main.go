// Package main is a Drone / Woodpecker plugin that writes pages to Gitea wiki
// via the REST API.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"codeberg.org/woodpecker-plugins/go-plugin"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type Settings struct {
	GiteaURL    string
	GiteaToken  string
	Repo        string
	Page        string
	Content     string
	ContentFile string
	WikiMode    string // skip (default), overwrite, append
}

type Plugin struct {
	*plugin.Plugin
	Settings *Settings
	client   *http.Client
	baseURL  string
}

func (p *Plugin) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name: "gitea-url", Usage: "Gitea instance URL (default: CI_FORGE_URL)",
			Sources: cli.EnvVars("PLUGIN_GITEA_URL", "CI_FORGE_URL"), Destination: &p.Settings.GiteaURL,
		},
		&cli.StringFlag{
			Name: "gitea-token", Usage: "Gitea API token with write:repository scope",
			Sources: cli.EnvVars("PLUGIN_GITEA_TOKEN"), Destination: &p.Settings.GiteaToken,
		},
		&cli.StringFlag{
			Name: "repo", Usage: "target repository (owner/repo)",
			Sources: cli.EnvVars("PLUGIN_REPO", "CI_REPO"), Destination: &p.Settings.Repo,
		},
		&cli.StringFlag{
			Name: "page", Usage: "wiki page title, e.g. Code-Review/2024-07-06",
			Sources: cli.EnvVars("PLUGIN_PAGE"), Destination: &p.Settings.Page,
		},
		&cli.StringFlag{
			Name: "content", Usage: "markdown content (inline; use content_file for large content)",
			Sources: cli.EnvVars("PLUGIN_CONTENT"), Destination: &p.Settings.Content,
		},
		&cli.StringFlag{
			Name: "content-file", Usage: "path to file containing markdown content",
			Sources: cli.EnvVars("PLUGIN_CONTENT_FILE"), Destination: &p.Settings.ContentFile,
		},
		&cli.StringFlag{
			Name: "wiki-mode", Usage: "existing page handling: skip (default), overwrite, append",
			Value: "skip", Sources: cli.EnvVars("PLUGIN_WIKI_MODE"), Destination: &p.Settings.WikiMode,
		},
	}
}

func (p *Plugin) Execute(ctx context.Context) error {
	s := p.Settings
	if err := s.validate(); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	if s.ContentFile != "" {
		data, err := os.ReadFile(s.ContentFile)
		if err != nil {
			return fmt.Errorf("read content_file %s: %w", s.ContentFile, err)
		}
		s.Content = string(data)
	}

	p.baseURL = strings.TrimRight(s.GiteaURL, "/")
	p.client = &http.Client{Timeout: 30 * time.Second}

	exists := p.pageExists(ctx)

	switch s.WikiMode {
	case "skip":
		if exists {
			log.Info().Str("page", s.Page).Msg("page exists, skipping (wiki_mode=skip)")
			return nil
		}
		return p.upsert(ctx)
	case "append":
		if exists {
			s.Content = p.getPageContent(ctx) + "\n\n---\n\n" + s.Content
		}
		return p.upsert(ctx)
	default: // overwrite
		return p.upsert(ctx)
	}
}

func (p *Plugin) payload() []byte {
	b := map[string]string{
		"title":          p.Settings.Page,
		"content_base64": base64.StdEncoding.EncodeToString([]byte(p.Settings.Content)),
	}
	j, _ := json.Marshal(b)
	return j
}

// upsert tries PATCH first, then falls back to POST.
func (p *Plugin) upsert(ctx context.Context) error {
	log.Info().Str("page", p.Settings.Page).Int("bytes", len(p.Settings.Content)).Msg("publishing")

	u := fmt.Sprintf("%s/api/v1/repos/%s/wiki/page/%s",
		p.baseURL, p.Settings.Repo, url.PathEscape(p.Settings.Page))
	status, body := p.do(ctx, http.MethodPatch, u, p.payload())
	if status == 200 || status == 201 {
		log.Info().Int("status", status).Msg("wiki page updated")
		return nil
	}
	if status != 404 {
		return fmt.Errorf("wiki PATCH returned %d: %s", status, body)
	}

	u = fmt.Sprintf("%s/api/v1/repos/%s/wiki/new", p.baseURL, p.Settings.Repo)
	status, body = p.do(ctx, http.MethodPost, u, p.payload())
	if status == 200 || status == 201 {
		log.Info().Int("status", status).Msg("wiki page created")
		return nil
	}
	return fmt.Errorf("wiki POST returned %d: %s", status, body)
}

// pageExists returns true if the wiki page already exists.
func (p *Plugin) pageExists(ctx context.Context) bool {
	u := fmt.Sprintf("%s/api/v1/repos/%s/wiki/page/%s",
		p.baseURL, p.Settings.Repo, url.PathEscape(p.Settings.Page))
	status, _ := p.do(ctx, http.MethodGet, u, nil)
	return status == 200
}

// getPageContent fetches and decodes the current content of the wiki page.
func (p *Plugin) getPageContent(ctx context.Context) string {
	u := fmt.Sprintf("%s/api/v1/repos/%s/wiki/page/%s",
		p.baseURL, p.Settings.Repo, url.PathEscape(p.Settings.Page))
	_, body := p.do(ctx, http.MethodGet, u, nil)
	var result struct {
		ContentBase64 string `json:"content_base64"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return ""
	}
	decoded, _ := base64.StdEncoding.DecodeString(result.ContentBase64)
	return string(decoded)
}

// do sends an HTTP request and returns status code + trimmed response body.
func (p *Plugin) do(ctx context.Context, method, url string, payload []byte) (int, string) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Authorization", "token "+p.Settings.GiteaToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(b))
}

func (s *Settings) validate() error {
	switch {
	case s.GiteaURL == "":
		return fmt.Errorf("gitea_url is required")
	case s.GiteaToken == "":
		return fmt.Errorf("gitea_token is required")
	case s.Repo == "":
		return fmt.Errorf("repo is required")
	case s.Page == "":
		return fmt.Errorf("page is required")
	case s.Content == "" && s.ContentFile == "":
		return fmt.Errorf("content or content_file is required")
	case s.WikiMode != "skip" && s.WikiMode != "overwrite" && s.WikiMode != "append":
		return fmt.Errorf("wiki_mode must be skip, overwrite, or append")
	}
	return nil
}

func main() {
	p := &Plugin{Settings: &Settings{}}
	p.Plugin = plugin.New(plugin.Options{
		Name: "plugin-gitea-wiki", Version: "0.1.0",
		Description: "Publish pages to Gitea wiki via REST API",
		Flags: p.Flags(), Execute: p.Execute,
	})
	p.Run()
}
