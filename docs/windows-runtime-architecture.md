# Windows runtime architecture

Family Friend uses three Windows process roles. Protection state and policy remain
authoritative in the Windows Service; user-session processes only provide
capabilities that Session 0 cannot access.

## Process boundary

```text
Remote control service
        |
        | HTTPS
        v
+-----------------------------+
| drowsyfriend Windows Service|
| Session 0                   |
|                             |
| - authoritative policy      |
| - KidControl decisions      |
| - browser localhost API     |
| - remote service sync       |
| - pairing/device identity   |
| - service-safe enforcement  |
+--------------+--------------+
               |
               | local Windows named-pipe RPC
               | bounded request/response
               v
+-----------------------------+
| hidden user-session worker  |
| configured child session    |
|                             |
| - active-window observation |
| - close-window action       |
| - future screen capture     |
| - no policy decisions       |
| - no remote credentials     |
+-----------------------------+

+-----------------------------+
| Fyne tray / management UI   |
| user session                |
|                             |
| presentation + user actions |
| lifecycle independent from  |
| protection/session worker   |
+-----------------------------+
```

## Windows Service ownership

The Windows Service owns all authoritative runtime state:

- loaded profile/policy state and revisions
- KidControl policy evaluation and decisions
- browser localhost API
- communication with the remote control service
- customer/profile/device identity and pairing state
- audit/event generation
- enforcement that does not require interactive-desktop access
- lifecycle across boot, logout, user switching, and UI closure

Only the service may decide whether an application/window/site is allowed. The
session worker and UI must not keep independent policy copies.

## User-session worker ownership

The hidden user-session worker exists only because Windows services run in
Session 0. It executes operations that require the child's interactive desktop:

- observe the foreground window
- perform close-window actions requested by the service
- later capture the interactive desktop/screen

It returns observations or action results. It does not decide policy, contact
the remote control service, store customer credentials, or expose the browser
API.

The worker lifecycle is independent from the Fyne UI. Closing the tray/window
must not stop the worker.

## IPC decision

Use a Windows named-pipe request/response boundary between the service and the
session worker.

Properties:

- local machine only; no TCP listener
- protocol is versioned
- requests are bounded by context/deadline
- the service is the only policy authority
- payloads contain only the minimum observation/action data
- no remote-service credentials cross the boundary
- pipe access is restricted using Windows identity/ACLs to the service and the
  configured child-session worker
- malformed, unauthenticated, stale, or wrong-version peers are rejected

For the MVP, expose only the operations needed by current KidControl:

```text
GetActiveWindow() -> window observation
CloseWindow(target) -> action result
```

Future screen-capture operations extend this protocol rather than creating a
second IPC mechanism.

The service-side implementation should adapt this client to the existing
`platform.WindowProvider` boundary so KidControl does not contain IPC-specific
logic.

## Session availability

When the configured child user is not logged in, or the session worker is
unavailable:

- the Windows Service remains running
- browser/API/remote-sync and other service-safe protection continue
- interactive-desktop observations/actions report unavailable
- the service must not use stale foreground-window data as if it were current
- calls fail within a bounded deadline; service shutdown must never wait
  indefinitely for the session worker

When the worker starts or restarts, the service reconnects automatically.
Because the worker owns no policy state, reconnect does not require policy
reconciliation.

## Multiple Windows sessions

MVP supports one configured child Windows session at a time.

Other interactive sessions are not used as substitutes. If the configured
child session is absent, interactive-desktop capability is unavailable rather
than silently attaching to another user.

Supporting multiple protected concurrent sessions requires a later extension
that identifies workers by Windows session/user identity and routes requests
explicitly.

## UI lifecycle

The Fyne tray/status application is a presentation surface only.

- it may open/close independently
- it talks to the local service API
- it does not host the authoritative KidControl runtime
- it does not host the hidden session worker
- closing it does not disable protection

## Implementation order

1. #150 provides the thin Windows Service host.
2. #158 fixes this ownership/IPC contract.
3. #159 implements the service-to-session named-pipe client/server boundary and
   the `platform.WindowProvider` adapter.
4. #160 implements the hidden child-session worker and login startup.
5. #151 may install/enable the production automatic Windows Service only after
   #159 and #160 preserve interactive-session monitoring.
6. Fyne UI work (#152-#155) remains independent of the protection lifecycle.
