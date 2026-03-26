# toc OAuth Callback Relay

A Cloudflare Worker that bridges Slack's HTTPS redirect requirement with the CLI's localhost callback server.

## How it works

Slack requires HTTPS redirect URIs. The CLI can only listen on localhost. This worker sits in between:

```
Slack OAuth redirect
  → https://toc.opencompany.cloud/slack/callback?code=xyz
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

## One-time infrastructure setup

These steps only need to be done once. After that, deployments are handled by CI.

### 1. Cloudflare API token

1. Go to [Cloudflare API Tokens](https://dash.cloudflare.com/profile/api-tokens)
2. Create a token with **Workers Scripts: Edit** permission, scoped to the account that owns `opencompany.cloud`
3. Add it as a GitHub repository secret named `CLOUDFLARE_API_TOKEN`

### 2. DNS / Worker route for `toc.opencompany.cloud`

The worker needs to be reachable at `https://toc.opencompany.cloud/slack/callback`. There are two options:

**Option A: Custom domain (recommended)**

1. In the Cloudflare dashboard, go to **Workers & Pages > toc-auth-callback > Settings > Domains & Routes**
2. Add a custom domain: `toc.opencompany.cloud`
3. Cloudflare automatically provisions the DNS record and TLS certificate

**Option B: Worker route**

1. In the Cloudflare dashboard, go to the **opencompany.cloud** zone > **Workers Routes**
2. Add a route: `toc.opencompany.cloud/slack/*` → `toc-auth-callback`
3. Ensure a DNS record exists for `toc.opencompany.cloud` (a proxied AAAA record to `100::` works as a placeholder)

The route pattern in `wrangler.toml` must match whichever option you choose.

### 3. Slack app redirect URI

In your [Slack app settings](https://api.slack.com/apps), add `https://toc.opencompany.cloud/slack/callback` as an allowed redirect URI under **OAuth & Permissions**.

## Local development

```bash
npx wrangler dev
```

Then visit `http://localhost:8787/slack/callback?code=test123` — you should get a 302 redirect to `http://localhost:8976/callback?code=test123`.
