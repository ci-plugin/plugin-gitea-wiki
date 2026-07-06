# plugin-gitea-wiki

Publish pages to Gitea wiki via REST API.

```yaml
steps:
  - name: publish-wiki
    image: ghcr.io/ci-plugin/plugin-gitea-wiki:latest
    settings:
      page: Code-Review/report
      content_file: report.md
      wiki_mode: overwrite
      gitea_token:
        from_secret: gitea_token
```

## Settings

`gitea_url` 和 `repo` 从 CI 环境自动获取。

| Setting | Required | Default | Description |
|---------|:--------:|---------|-------------|
| `gitea_token` | ✅ | — | API token with repo write access |
| `page` | ✅ | — | Wiki page title |
| `content` | * | — | Markdown content (inline) |
| `content_file` | * | — | Path to file containing markdown content |
| `wiki_mode` | — | `skip` | `skip` / `overwrite` / `append` |
| `gitea_url` | — | `CI_FORGE_URL` | Gitea instance URL |
| `repo` | — | `CI_REPO` | Target repository (owner/repo) |

*`content` 或 `content_file` 二选一。

## Wiki Modes

| Mode | 页面已存在 | 页面不存在 |
|------|-----------|-----------|
| `skip` (默认) | 跳过，不修改 | 创建新页面 |
| `overwrite` | 覆盖内容 | 创建新页面 |
| `append` | 追加到现有内容末尾 | 创建新页面 |
