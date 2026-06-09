# Web Access on a VPS

Web Access is the first Web-facing slice of CoWork OS. It is intentionally scoped to the
agent task loop: choose a server-side workspace, create a task, follow task events, and send
follow-up messages from a desktop or mobile browser.

It does not attempt to mirror every Electron desktop feature. File browsing, terminal tabs,
Browser Workbench, Finder integration, tray controls, and OS keychain behavior still need
dedicated server-side implementations or desktop fallbacks.

## Build

```bash
npm install
npm run build:react
npm run build:daemon
```

The daemon WebAccess host serves the built renderer from `dist/renderer` by default. If you
package or deploy assets elsewhere, set `COWORK_WEB_ACCESS_RENDERER_PATH`.

## Run

```bash
COWORK_WEB_ACCESS=1 \
COWORK_WEB_ACCESS_HOST=127.0.0.1 \
COWORK_WEB_ACCESS_PORT=3847 \
COWORK_WEB_ACCESS_TOKEN="$(openssl rand -hex 32)" \
COWORK_BOOTSTRAP_WORKSPACE_PATH=/srv/cowork/workspace \
node dist/daemon/daemon/main.js --headless
```

Open `http://127.0.0.1:3847/?token=<token>` through an SSH tunnel, Tailscale, or a trusted
HTTPS reverse proxy.

## Reverse Proxy

Use HTTPS before exposing WebAccess beyond localhost. WebAccess includes bearer-token auth
and origin checks, but it can create tasks and send messages to the agent runtime, so it
should be treated as an operator surface.

Example Caddy route:

```caddyfile
cowork.example.com {
  reverse_proxy 127.0.0.1:3847
}
```

If the browser origin differs from the WebAccess origin, set:

```bash
COWORK_WEB_ACCESS_ALLOWED_ORIGINS=https://cowork.example.com
```

## Runtime Surface

Available in the Web MVP:

- `GET /api/health`
- `GET /api/capabilities`
- `GET /api/workspaces`
- `GET /api/tasks`
- `POST /api/tasks`
- `GET /api/tasks/:id`
- `GET /api/tasks/:id/events`
- `POST /api/tasks/:id/message`
- `GET /api/accounts`
- `GET /api/accounts/:id`
- `GET /ws?token=...` for live daemon events

Planned local-capability services:

- File browser and artifact preview: server-side workspace file API with path sandboxing.
- Terminal tabs: WebSocket PTY sessions with audit and permission checks.
- Browser Workbench: remote Playwright/CDP browser service replacing Electron `webview`.
- Desktop shell features: remain desktop-only or map to browser/PWA equivalents.

## Mobile Use

The WebAccess page is mobile-first and includes a minimal PWA manifest. On iOS or Android,
open the HTTPS URL, connect with the access token, then add the page to the home screen.

Keep the token private. Anyone with the token can operate the exposed WebAccess task API.
