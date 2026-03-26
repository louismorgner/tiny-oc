# Integrations

Integrations connect agents to external APIs through a controlled gateway. Each integration defines available actions, authentication, rate limits, and response filtering. Agents access them via `toc runtime invoke` during sessions.

## How it works

The integration system has four layers:

1. **Definitions** — YAML files describing an API's actions, auth method, endpoints, and rate limits
2. **Credentials** — encrypted tokens stored locally, decrypted at invocation time
3. **Permissions** — scoped access rules declared in `oc-agent.yaml` and enforced at the gateway
4. **Gateway** — the runtime path that checks permissions, loads credentials, enforces rate limits, makes the HTTP call, and filters the response

When an agent calls `toc runtime invoke github issues.read --repo owner/repo`, the gateway:

1. Loads the permission manifest for the current session
2. Checks whether `issues.read` is allowed for the requested scope
3. Loads and decrypts the GitHub credential
4. Checks the per-session rate limit
5. Builds and sends the HTTP request
6. Filters the response to whitelisted fields
7. Logs the invocation to the audit trail

## Adding an integration

```bash
toc integrate add github
```

For token-based integrations (GitHub), you'll be prompted for a personal access token. For OAuth2-based integrations (Slack), toc opens a browser for the authorization flow and runs a local callback server to capture the token.

```bash
toc integrate list       # list configured integrations
toc integrate test github  # verify credentials work
toc integrate remove github
```

## Credential storage

Credentials are encrypted with AES-256-GCM. The master encryption key is stored in the macOS keychain (service: `toc-integrations`, account: `master-key`).

Encrypted credentials live at:

- `<workspace>/.toc/integrations/<name>/credentials.enc` — stored at `toc integrate add` time, read directly by sessions at invocation time (no per-session copy is made)

For OAuth2 integrations, client config is stored separately at `~/.toc/integrations/<name>/oauth2_client.enc` to support automatic token refresh.

## Permissions

Integration permissions are declared in the agent's `oc-agent.yaml` under `permissions.integrations`:

```yaml
permissions:
  integrations:
    github:
      - "issues.read:*"
      - "issues.write:louismorgner/tiny-oc"
      - "pulls.read:*"
    slack:
      - "send_message:#engineering"
      - "read_messages:*"
```

Each permission follows the format `action:scope`:

| Component | Description | Examples |
|---|---|---|
| `action` | The integration action name | `issues.read`, `send_message`, `pulls.write` |
| `scope` | Target resource the action applies to | `*` (any), `owner/repo`, `#channel` |

Permissions are resolved at spawn time into a manifest at `.toc/sessions/<id>/permissions.json`. The gateway checks this manifest on every invocation. **Default deny** — if an integration or action isn't listed, access is blocked.

## Rate limiting

Each action can define a rate limit in its integration definition:

```yaml
rate_limit:
  max: 30
  window: 60s
```

Rate limits are tracked per-session in `.toc/sessions/<id>/rate_limits.json`. When the limit is exceeded, the gateway rejects the call until the window resets.

## Integration definitions

Integration definitions are YAML files that describe an API. They live in `registry/integrations/<name>/integration.yaml`.

```yaml
name: github
description: GitHub API — issues, pull requests, and repositories
auth:
  method: token              # token, api_key, or oauth2
  setup_url: https://github.com/settings/tokens
  required_scopes:
    - repo
    - read:org

actions:
  issues.read:
    description: Read issues from a repository
    scopes:
      "*": any repository
    params:
      - name: repo
        required: true
      - name: state
        required: false
        default: open
    method: GET
    endpoint: https://api.github.com/repos/{{repo}}/issues
    auth_header: "Bearer {{token}}"
    body_format: query
    rate_limit:
      max: 30
      window: 60s
    returns:
      - "[].number"
      - "[].title"
      - "[].state"
```

### Definition fields

| Field | Description |
|---|---|
| `name` | Integration identifier |
| `description` | Human-readable description |
| `auth.method` | Authentication type: `token`, `api_key`, or `oauth2` |
| `auth.setup_url` | Where users create credentials |
| `auth.required_scopes` | OAuth scopes or token permissions needed |
| `auth.setup_instructions` | Optional multi-line setup guide |
| `actions` | Map of action definitions (see below) |

### Action fields

| Field | Description |
|---|---|
| `description` | What the action does |
| `scopes` | Map of scope values to descriptions |
| `params` | List of parameters (`name`, `required`, `default`) |
| `method` | HTTP method: `GET`, `POST`, `PUT`, `DELETE`, `PATCH` |
| `endpoint` | URL with `{{param}}` placeholders |
| `auth_header` | Authorization header with `{{token}}` placeholder |
| `body_format` | How non-URL params are sent: `json`, `query`, or `form` |
| `rate_limit` | Optional `max` calls per `window` duration |
| `returns` | Optional response field whitelist (dot notation, `[].field` for arrays) |

## Built-in integrations

### GitHub

Auth: Personal access token. Scopes: `repo`, `read:org`.

| Action | Description | Key params |
|---|---|---|
| `issues.read` | List repository issues | `repo`, `state`, `per_page` |
| `issues.write` | Create an issue | `repo`, `title`, `body` |
| `issues.comment` | Comment on an issue | `repo`, `issue_number`, `body` |
| `pulls.read` | List pull requests | `repo`, `state`, `per_page` |
| `pulls.comment` | Add a PR review | `repo`, `pull_number`, `body` |
| `repos.read` | Get repository info | `repo` |

### Slack

Auth: Bot User OAuth Token (`xoxb-`). Scopes: `channels:read`, `channels:history`, `chat:write`, `reactions:write`.

| Action | Description | Key params |
|---|---|---|
| `send_message` | Send a message | `channel`, `text` |
| `read_messages` | Read channel history | `channel`, `limit` |
| `list_channels` | List public channels | `limit`, `types` |
| `react` | Add an emoji reaction | `channel`, `timestamp`, `name` |

## Using integrations from agents

During a session, agents call integrations through `toc runtime invoke`:

```bash
toc runtime invoke github issues.read --repo louismorgner/tiny-oc --state open
toc runtime invoke slack send_message --channel "#engineering" --text "Deploy complete"
```

The agent's CLAUDE.md or system prompt should document which integrations are available and how to use them. The gateway handles authentication and permission enforcement transparently.

## Audit trail

Every integration invocation is logged to `.toc/audit.log` with action `runtime.invoke`, including the integration name, action, target scope, and HTTP status code.
