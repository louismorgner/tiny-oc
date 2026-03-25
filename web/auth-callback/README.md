# toc OAuth Callback Relay

A Cloudflare Worker that receives OAuth callbacks from Slack and displays the authorization code for the user to copy back into the toc CLI.

## What it does

Slack requires HTTPS redirect URIs. This worker handles the redirect at `https://auth.opencompany.cloud/slack/callback`, extracts the `?code=` parameter, and displays it in a clean page so the user can copy-paste it into their terminal.

No codes are stored, logged, or transmitted anywhere. The worker is completely stateless.

## Deploy

Prerequisites: [Wrangler CLI](https://developers.cloudflare.com/workers/wrangler/install-and-update/) and a Cloudflare account.

```bash
cd web/auth-callback
npx wrangler deploy
```

The worker serves a single route: `https://auth.opencompany.cloud/slack/callback`

You'll need to configure the `auth.opencompany.cloud` subdomain in Cloudflare DNS to point to the worker.

## Local development

```bash
npx wrangler dev
```

Then visit `http://localhost:8787/slack/callback?code=test123` to preview the page.
