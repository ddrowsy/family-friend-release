# Windows MSI

This directory contains the WiX foundation for the Family Friend x64 Windows installer.

## Build

Run from a Windows checkout:

```powershell
powershell -ExecutionPolicy Bypass -File installer/windows/build.ps1
```

Use `-OutputDirectory` to override the default output directory:

```powershell
powershell -ExecutionPolicy Bypass -File installer/windows/build.ps1 -OutputDirectory artifacts/windows
```

The script:

1. builds the production Windows binaries for `windows/amd64`,
2. stages only the three customer runtime executables,
3. restores the repository-pinned WiX 6.0.2 local tool,
4. builds and validates the MSI,
5. decompiles the MSI and verifies its file table contains exactly the intended executables.

Output:

```text
dist/windows/FamilyFriend-x64.msi
```

The MSI installs these files under `C:\Program Files\Family Friend\`:

- `drowsyfriend.exe`
- `drowsyfriend-ui.exe`
- `drowsyfriend-session-worker.exe`

`drowsyfriendctl.exe` is intentionally excluded.

## Prerequisites

- Windows x64
- Go 1.27.1
- .NET SDK supported by WiX 6
- a Windows C compiler available to Go/CGO for the production Fyne desktop UI build

WiX cannot build MSI packages on the existing Linux `oracle-micro` runner because MSI creation depends on Windows Installer components outside .NET. Linux/WSL can continue to run the existing cross-build verification, but the MSI build itself must run on Windows.

Service registration, login/session startup wiring, upgrades, signing, and release publishing are intentionally left to follow-up issues.
