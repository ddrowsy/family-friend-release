# drowsyfriend

drowsyfriend is a Windows-first local device-control project. The first product module is `kidcontrol`, which monitors and controls selected child-device activity from a JSON policy.

The project currently includes app/window monitoring, browser policy profiles, a localhost browser API, and a Chrome/Chromium Manifest V3 extension.

## Current architecture

The runtime is intentionally small:

```text
cmd/drowsyfriend
  -> internal/app.Runtime
  -> internal/modules.Registry
  -> internal/modules/kidcontrol.Module
       -> app monitor
       -> window monitor
       -> screen monitor
       -> browser monitor
       -> decision/action helpers
       -> audit
  -> internal/platform
       -> internal/platform/windows
```

KidControl implementation stays primarily in one Go package. Platform-specific operating-system behavior remains behind `internal/platform`.

The browser extension is deliberately thin:

```text
Chrome browser events
  -> browser-extension/chrome
  -> http://127.0.0.1:17653
  -> KidControl BrowserMonitor
  -> current browser profile
  -> Go URL policy evaluator
  -> allow/block decision
```

Go is the source of truth for browser profiles, URL matching, temporary profile expiry/fallback, visit state, and audit logging. The extension observes browser activity and enforces the decision returned by KidControl; it does not duplicate policy rules.

## Browser control

Browser monitoring supports named profiles with:

- `allow_list` or `block_list` policy
- `allowed_urls`
- `blocked_urls`
- `block_inappropriate` configuration
- mutable profiles at runtime
- temporary profile activation with duration and fallback profile

`block_inappropriate` is currently configuration only. No inappropriate-content classifier is implemented yet.

The localhost browser API listens on `127.0.0.1:17653` and currently exposes:

- `GET /api/browser/profile`
- `POST /api/browser/check`
- `POST /api/browser/end-visit`

Browser visits are timed in Go. Switching active tabs, closing the active tab, or losing browser focus ends the current visit. Blocked navigation attempts are audited immediately.

If the local daemon/API is unavailable, the current Chrome extension fails open rather than blocking browsing.

Browser monitoring is disabled in `configs/default.policy.json` by default.

## Chrome extension

The extension lives in `browser-extension/chrome` and uses Manifest V3.

Important files include:

- `manifest.json` - extension declaration and localhost permission
- `background.js` - Chrome event wiring
- `navigation.js` - navigation filtering/decision orchestration
- `activity.js` - active-tab and browser-focus visit boundaries
- `kidcontrol_api.js` - localhost API client
- `blocked.html` / `blocked.js` - extension-owned block page
- `smoke.e2e.js` - Playwright/Chromium integration smoke test

Node's built-in test runner is used for extension unit tests. Playwright is only used for the Chromium smoke test.

## Development

The Go module currently targets Go 1.27.1.

Common local commands are provided by `make.sh`:

```bash
./make.sh test
./make.sh run
./make.sh build
./make.sh build-windows
./make.sh ctl-validate
./make.sh ctl-modules
```

The full CI verification can also be run with:

```bash
bash scripts/test-ci.sh
```

That script runs verification in Docker and checks:

1. `gofmt`
2. `go test ./...`
3. browser-extension unit tests
4. Chromium extension smoke test
5. `git diff --check`

## Current product boundaries

Keep the implementation practical:

- Windows 11 is the first platform target.
- Keep KidControl behavior in the existing `kidcontrol` package unless a real boundary requires otherwise.
- Do not reintroduce Service/Engine/decision/action subpackage layers only for abstraction.
- Keep browser policy authoritative in Go.
- Do not cache or duplicate browser profile policy in JavaScript.
- Reuse the current audit path unless a dedicated audit feature is intentionally designed.
- Prefer focused issues and small PRs over speculative architecture.

## License

Family Friend is **source-available**, not OSI open source. The source is published for transparency, security and audit review, learning, personal use, and other purposes permitted by the [PolyForm Noncommercial License 1.0.0](LICENSE).

You may inspect, fork, modify, and redistribute the software only as allowed by that license. Commercial use requires separate permission from the project owner/licensor.

Third-party components remain subject to their own license terms. The repository license grants only rights the project owner/licensor is entitled to grant.
