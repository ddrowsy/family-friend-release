#!/usr/bin/env bash
set -euo pipefail

BASE_REF="${1:-origin/main}"
FULL_VERIFICATION="${FULL_VERIFICATION:-false}"
FORCE_FULL="${FORCE_FULL:-false}"

GO_CHANGED=false
BROWSER_CHANGED=false
INSTALLER_CHANGED=false

if [[ "$FORCE_FULL" == "true" ]]; then
  FULL_VERIFICATION=true
fi

while IFS= read -r path; do
  case "$path" in
    *.go|go.mod|go.sum)
      GO_CHANGED=true
      ;;
  esac

  case "$path" in
    browser-extension/chrome/*)
      BROWSER_CHANGED=true
      ;;
  esac

  case "$path" in
    installer/windows/*|scripts/test-windows-installer-source.sh)
      INSTALLER_CHANGED=true
      ;;
  esac

  case "$path" in
    go.mod|go.sum|internal/policy/*|.github/workflows/*|.github/actions/*|scripts/test-ci.sh|scripts/test-go-impacted.sh|scripts/test-windows-build.sh|scripts/ci-classify.sh)
      FULL_VERIFICATION=true
      ;;
  esac
done < <(git diff --name-only "$BASE_REF"...HEAD)

printf 'GO_CHANGED=%s\n' "$GO_CHANGED"
printf 'BROWSER_CHANGED=%s\n' "$BROWSER_CHANGED"
printf 'INSTALLER_CHANGED=%s\n' "$INSTALLER_CHANGED"
printf 'FULL_VERIFICATION=%s\n' "$FULL_VERIFICATION"
