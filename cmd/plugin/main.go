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
	"net/url"
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
			Usage:       "Gitea instance URL (default: CI_FORGE_URL)",
			Sources:     cli.EnvVars("PLUGIN_GITEA_URL", "CI_FORGE_URL"),
			Destination: &p.Settings.GiteaURL,
		},
		&cli.StringFlag{
			Name:        "gitea-token",
			Usage:       "Gitea API token with write:repository scope",
			Sources:     cli.EnvVars("PLUGIN_GITEA_TOKEN"),
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

	log.Info().Int("content_len", len(s.Content)).Msg("content to publish")

	// Gitea wiki API: content must be base64-encoded
	body := map[string]string{
		"title":   s.Page,
		"content_base64": base64.StdEncoding.EncodeToString([]byte(s.Content)),
	}
	payload, _ := json.Marshal(body)

	log.Info().
		Str("title", body["title"]).
		Int("content_bytes", len(s.Content)).
		Int("b64_len", len(body["content_base64"])).
		Str("b64_head", body["content_base64"][:min(80, len(body["content_base64"]))]).
		Msg("request payload")

	baseURL := strings.TrimRight(s.GiteaURL, "/")
	client := &http.Client{Timeout: 30 * time.Second}

	// Try PATCH (update) first, fall back to POST (create)
	patchURL := fmt.Sprintf("%s/api/v1/repos/%s/wiki/%s",
		baseURL, s.Repo, url.PathEscape(s.Page))
	log.Info().Str("url", patchURL).Str("page", s.Page).Msg("updating wiki page")

	req, _ := http.NewRequestWithContext(ctx, http.MethodPatch, patchURL, bytes.NewReader(payload))
	req.Header.Set("Authorization", "token "+s.GiteaToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("api call: %w", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		log.Info().Int("status", resp.StatusCode).Msg("wiki page updated")
		return nil
	}

	// PATCH failed — create new page
	log.Info().Int("status", resp.StatusCode).Str("response", strings.TrimSpace(string(respBody))).Msg("patch failed, trying POST")

	createURL := fmt.Sprintf("%s/api/v1/repos/%s/wiki/new", baseURL, s.Repo)
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(payload))
	req.Header.Set("Authorization", "token "+s.GiteaToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ = io.ReadAll(resp.Body)

	log.Info().Int("status", resp.StatusCode).Str("response", strings.TrimSpace(string(respBody))).Msg("wiki create response")

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		log.Info().Str("page", s.Page).Msg("wiki page created")
		return nil
	}

	return fmt.Errorf("wiki API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}

func (s *Settings) validate() error {
	if s.GiteaURL == "" {
		return fmt.Errorf("gitea_url is required")
	}
	if s.GiteaToken == "" {
		return fmt.Errorf("gitea_token is required")
	}
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
