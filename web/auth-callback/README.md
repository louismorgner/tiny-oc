# toc OAuth Callback Relay

A Cloudflare Worker that bridges Slack's HTTPS redirect requirement with the CLI's localhost callback server.

## How it works

Slack requires HTTPS redirect URIs. The CLI can only listen on localhost. This worker sits in between:

```
Slack OAuth redirect
  → https://square-paper-84df.dev-f64.workers.dev/slack/callback?code=xyz
  → Worker 302 redirects to http://localhost:8976/callback?code=xyz
  → CLI's localhost server captures the code automatically
```

No codes are stored, logged, or transmitted anywhere. The worker is stateless — it forwards the query string and nothing else.

For headless/SSH environments where localhost isn't reachable, use `toc integrate add slack --manual` to copy-paste the code from the browser URL bar instead.

## Deployment

Deploys are automatic via GitHub Actions on push to `main` when files in `web/auth-callback/` change. See `.github/workflows/deploy-worker.yaml`.

Manual deploy (if needed):

```bash
cd web/auth-callback
npx wrangler deploy
```

## One-time setup

### Cloudflare API token

1. Go to [Cloudflare API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. Create a token with **Workers Scripts: Edit** permission
3. Add it as a GitHub repository secret named `CLOUDFLARE_API_TOKEN`

### Slack app redirect URI

In your [Slack app settings](https://api.slack.com/apps), add `https://square-paper-84df.dev-f64.workers.dev/slack/callback` as an allowed redirect URI under **OAuth & Permissions**.

## Local development

```bash
npx wrangler dev
```

Then visit `http://localhost:8787/slack/callback?code=test123` — you should get a 302 redirect to `http://localhost:8976/callback?code=test123`.
