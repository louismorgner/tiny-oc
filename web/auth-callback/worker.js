// Cloudflare Worker: OAuth callback relay for toc CLI
// Extracts the authorization code from Slack's OAuth redirect and displays it
// for the user to copy back into their terminal. No server-side storage or logging.

export default {
  async fetch(request) {
    const url = new URL(request.url);

    if (url.pathname !== "/slack/callback") {
      return new Response("Not found", { status: 404 });
    }

    const error = url.searchParams.get("error");
    const errorDescription = url.searchParams.get("error_description") || "";
    const code = url.searchParams.get("code");

    if (error) {
      return new Response(errorPage(error, errorDescription), {
        status: 400,
        headers: { "Content-Type": "text/html;charset=UTF-8" },
      });
    }

    if (!code) {
      return new Response(errorPage("missing_code", "No authorization code was included in the callback."), {
        status: 400,
        headers: { "Content-Type": "text/html;charset=UTF-8" },
      });
    }

    return new Response(successPage(code), {
      status: 200,
      headers: { "Content-Type": "text/html;charset=UTF-8" },
    });
  },
};

function successPage(code) {
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>toc - Authorization Successful</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: #0f172a;
      color: #e2e8f0;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 1rem;
    }
    .card {
      background: #1e293b;
      border: 1px solid #334155;
      border-radius: 12px;
      padding: 2.5rem;
      max-width: 520px;
      width: 100%;
      text-align: center;
    }
    .logo { font-size: 1.25rem; font-weight: 700; color: #38bdf8; margin-bottom: 1.5rem; letter-spacing: -0.02em; }
    h1 { font-size: 1.5rem; font-weight: 600; margin-bottom: 0.5rem; color: #f1f5f9; }
    .subtitle { color: #94a3b8; margin-bottom: 1.5rem; line-height: 1.5; }
    .code-box {
      background: #0f172a;
      border: 1px solid #475569;
      border-radius: 8px;
      padding: 1rem;
      font-family: "SF Mono", "Fira Code", "Fira Mono", Menlo, monospace;
      font-size: 0.875rem;
      word-break: break-all;
      color: #38bdf8;
      cursor: pointer;
      position: relative;
      transition: border-color 0.15s;
    }
    .code-box:hover { border-color: #38bdf8; }
    .copy-btn {
      margin-top: 1rem;
      background: #38bdf8;
      color: #0f172a;
      border: none;
      border-radius: 8px;
      padding: 0.75rem 1.5rem;
      font-size: 0.9375rem;
      font-weight: 600;
      cursor: pointer;
      transition: background 0.15s;
    }
    .copy-btn:hover { background: #7dd3fc; }
    .instructions {
      margin-top: 1.5rem;
      padding-top: 1.5rem;
      border-top: 1px solid #334155;
      color: #94a3b8;
      font-size: 0.875rem;
      line-height: 1.6;
    }
    .copied { color: #4ade80; font-weight: 600; }
  </style>
</head>
<body>
  <div class="card">
    <div class="logo">opencompany.cloud / toc</div>
    <h1>Authorization successful</h1>
    <p class="subtitle">Copy the code below and paste it into your terminal.</p>
    <div class="code-box" id="code" onclick="copyCode()" title="Click to copy">${escapeHtml(code)}</div>
    <button class="copy-btn" id="copyBtn" onclick="copyCode()">Copy code</button>
    <div class="instructions">
      <p>Return to your terminal where <strong>toc integrate add slack</strong> is waiting, and paste this code when prompted.</p>
      <p style="margin-top: 0.5rem;">You can close this tab after pasting.</p>
    </div>
  </div>
  <script>
    function copyCode() {
      const code = document.getElementById("code").textContent;
      navigator.clipboard.writeText(code).then(() => {
        const btn = document.getElementById("copyBtn");
        btn.textContent = "Copied!";
        btn.classList.add("copied");
        btn.style.background = "#065f46";
        btn.style.color = "#4ade80";
        setTimeout(() => {
          btn.textContent = "Copy code";
          btn.classList.remove("copied");
          btn.style.background = "";
          btn.style.color = "";
        }, 2000);
      });
    }
  </script>
</body>
</html>`;
}

function errorPage(error, description) {
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>toc - Authorization Failed</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: #0f172a;
      color: #e2e8f0;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 1rem;
    }
    .card {
      background: #1e293b;
      border: 1px solid #334155;
      border-radius: 12px;
      padding: 2.5rem;
      max-width: 520px;
      width: 100%;
      text-align: center;
    }
    .logo { font-size: 1.25rem; font-weight: 700; color: #38bdf8; margin-bottom: 1.5rem; letter-spacing: -0.02em; }
    h1 { font-size: 1.5rem; font-weight: 600; margin-bottom: 0.5rem; color: #fca5a5; }
    .error { color: #94a3b8; margin-top: 1rem; line-height: 1.5; }
    .error code { background: #0f172a; padding: 0.125rem 0.375rem; border-radius: 4px; font-size: 0.875rem; }
  </style>
</head>
<body>
  <div class="card">
    <div class="logo">opencompany.cloud / toc</div>
    <h1>Authorization failed</h1>
    <p class="error"><code>${escapeHtml(error)}</code></p>
    <p class="error">${escapeHtml(description)}</p>
    <p class="error" style="margin-top: 1.5rem;">Return to your terminal and try again.</p>
  </div>
</body>
</html>`;
}

function escapeHtml(str) {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}
