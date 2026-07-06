# plugin-gitea-wiki

Publish pages to Gitea wiki via REST API.

```yaml
steps:
  - name: publish-wiki
    image: ghcr.io/ci-plugin/plugin-gitea-wiki:latest
    settings:
      page: Home
      content: |
        # Welcome
        ...
      gitea_token:
        from_secret: gitea_token
```

## Settings

`gitea_url` 和 `repo` 从 CI 环境自动获取。

| Setting | Required | Default | Description |
|---------|:--------:|---------|-------------|
| `gitea_token` | ✅ | — | API token with repo write access |
| `page` | ✅ | — | Wiki page title |
| `content` | ✅ | — | Markdown content (base64-encoded internally) |
| `gitea_url` | — | `CI_SYSTEM_URL` | Gitea instance URL |
| `repo` | — | `CI_REPO` | Target repository (owner/repo) |
