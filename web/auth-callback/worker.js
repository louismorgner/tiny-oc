// Cloudflare Worker: OAuth callback relay for toc CLI
// Receives the authorization code from Slack's HTTPS redirect and bounces
// the user back to the CLI's localhost server where the code is captured
// automatically. No codes are stored or logged.

const LOCALHOST_CALLBACK = "http://localhost:8976/callback";

export default {
  async fetch(request) {
    const url = new URL(request.url);

    if (url.pathname !== "/slack/callback") {
      return new Response("Not found", { status: 404 });
    }

    // Forward the full query string (code, state, error, etc.) to localhost.
    const target = `${LOCALHOST_CALLBACK}?${url.searchParams.toString()}`;

    return Response.redirect(target, 302);
  },
};
