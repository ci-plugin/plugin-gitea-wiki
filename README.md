# plugin-gitea-wiki

Sync markdown content to Gitea wiki pages — wiki is just a git repo.

```yaml
steps:
  - name: sync-wiki
    image: ghcr.io/byte-me-labs/plugin-gitea-wiki:latest
    settings:
      gitea_url: https://gitea.example.com
      wiki_repo: owner/repo
      page: code-review/2024-07-06.md
      content: |
        # Code Review Report
        ...
      gitea_token:
        from_secret: gitea_token
```

## Pairing with plugin-open-code-review

```yaml
steps:
  - name: code-review
    image: ghcr.io/byte-me-labs/plugin-open-code-review:latest
    settings:
      mode: scan
      format: json
    environment:
      DEEPSEEK_API_KEY:
        from_secret: deepseek_api_key

  - name: sync-wiki
    image: ghcr.io/byte-me-labs/plugin-gitea-wiki:latest
    settings:
      gitea_url: https://gitea.example.com
      wiki_repo: owner/im-server
      page: code-review/auth-service.md
      content: |
        # Auth Service Review
        ...
      gitea_token:
        from_secret: gitea_token
```

## Settings

| Setting | Required | Default | Description |
|---------|----------|---------|-------------|
| `gitea_url` | yes | — | Gitea instance URL |
| `gitea_token` | yes | — | API token with repo write access |
| `wiki_repo` | yes | `CI_REPO` | Target repo (owner/repo) |
| `page` | yes | — | Wiki page path, supports subdirectories |
| `content` | yes | — | Markdown content |
| `message` | no | auto | Commit message |
| `git_user` | no | `plugin-gitea-wiki` | Commit author |
| `git_email` | no | `plugin@ci` | Commit author email |
