#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_REF="${BASE_REF:-origin/main}"
FULL_VERIFICATION="${FULL_VERIFICATION:-false}"
FORCE_FULL="${FORCE_FULL:-false}"
GO_CHANGED=false
BROWSER_CHANGED=false
INSTALLER_CHANGED=false

GO_IMAGE="golang:1.27.1-bookworm"
NODE_IMAGE="node:22-bookworm-slim"
PLAYWRIGHT_IMAGE="mcr.microsoft.com/playwright:v1.63.0-noble"

GO_MOD_CACHE="family-friend-go-mod"
GO_BUILD_CACHE="family-friend-go-build"
NPM_CACHE="family-friend-npm-cache"
NODE_MODULES="family-friend-playwright-node-modules"

cd "$REPO_ROOT"
git fetch origin main --no-tags

eval "$(
  FULL_VERIFICATION="$FULL_VERIFICATION" \
  FORCE_FULL="$FORCE_FULL" \
  bash scripts/ci-classify.sh "$BASE_REF"
)"

echo
echo "========================================"
echo "Environment"
echo "========================================"
echo "Repository: $REPO_ROOT"
echo "Commit: $(git rev-parse --short HEAD)"
echo "Requested full verification: $FULL_VERIFICATION"
echo "Go changed: $GO_CHANGED"
echo "Browser changed: $BROWSER_CHANGED"
echo "Installer changed: $INSTALLER_CHANGED"
docker version --format 'Docker: {{.Server.Version}}'
docker run --rm "$GO_IMAGE" go version
docker run --rm "$NODE_IMAGE" node --version

if [[ "$FULL_VERIFICATION" != "true" && "$GO_CHANGED" == "true" ]]; then
  go_mode="$(
    docker run --rm       -v "$REPO_ROOT:/src:ro"       -v "$GO_MOD_CACHE:/go/pkg/mod"       -v "$GO_BUILD_CACHE:/root/.cache/go-build"       -w /src       "$GO_IMAGE"       bash -euc '
        git config --global --add safe.directory /src
        bash scripts/test-go-impacted.sh "$1" --mode-only
      ' -- "$BASE_REF"
  )"

  echo "Go verification mode: $go_mode"
  if [[ "$go_mode" == "full" ]]; then
    FULL_VERIFICATION=true
  fi
fi

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  echo "full=$FULL_VERIFICATION" >> "$GITHUB_OUTPUT"
fi

if [[ "$FULL_VERIFICATION" == "true" || "$GO_CHANGED" == "true" ]]; then
  echo
  echo "========================================"
  echo "Go formatting"
  echo "========================================"

  docker run --rm     -v "$REPO_ROOT:/src:ro"     -w /src     -e FULL_VERIFICATION="$FULL_VERIFICATION"     -e BASE_REF="$BASE_REF"     "$GO_IMAGE"     bash -euc '
      git config --global --add safe.directory /src

      if [[ "$FULL_VERIFICATION" == "true" ]]; then
        mapfile -t files < <(git ls-files "*.go")
      else
        mapfile -t files < <(
          git diff --name-only --diff-filter=ACMR "$BASE_REF"...HEAD -- "*.go"
        )
      fi

      if [[ "${#files[@]}" -eq 0 ]]; then
        echo "No Go files to format-check."
        exit 0
      fi

      bad="$(gofmt -l "${files[@]}")"
      if [[ -n "$bad" ]]; then
        echo "Unformatted Go files:"
        echo "$bad"
        exit 1
      fi
    '

  echo "PASS: gofmt"

  echo
  echo "========================================"
  echo "Go tests"
  echo "========================================"

  if [[ "$FULL_VERIFICATION" == "true" ]]; then
    docker run --rm       -v "$REPO_ROOT:/src:ro"       -v "$GO_MOD_CACHE:/go/pkg/mod"       -v "$GO_BUILD_CACHE:/root/.cache/go-build"       -w /src       "$GO_IMAGE"       go test ./...
    echo "PASS: go test ./..."
  else
    docker run --rm       -v "$REPO_ROOT:/src:ro"       -v "$GO_MOD_CACHE:/go/pkg/mod"       -v "$GO_BUILD_CACHE:/root/.cache/go-build"       -w /src       "$GO_IMAGE"       bash -euc '
        git config --global --add safe.directory /src
        bash scripts/test-go-impacted.sh "$1"
      ' -- "$BASE_REF"
    echo "PASS: impacted Go tests"
  fi
fi

if [[ "$FULL_VERIFICATION" == "true" ]]; then
  echo
  echo "========================================"
  echo "Public source license/provenance audit"
  echo "========================================"

  docker run --rm \
    -v "$REPO_ROOT:/src:ro" \
    -v "$GO_MOD_CACHE:/go/pkg/mod" \
    -v "$GO_BUILD_CACHE:/root/.cache/go-build" \
    -w /src \
    "$GO_IMAGE" \
    bash scripts/audit-public-release.sh
fi

echo
echo "========================================"
echo "Public snapshot release tests"
echo "========================================"

bash scripts/test-public-snapshot.sh

echo "PASS: public snapshot release tests"

echo
echo "========================================"
echo "Public CodeQL workflow tests"
echo "========================================"

bash scripts/test-public-codeql.sh

echo "PASS: public CodeQL workflow tests"

if [[ "$FULL_VERIFICATION" == "true" || "$INSTALLER_CHANGED" == "true" ]]; then
  echo
  echo "========================================"
  echo "Windows installer source"
  echo "========================================"

  bash scripts/test-windows-installer-source.sh

  echo
  echo "========================================"
  echo "Windows agent build"
  echo "========================================"

  docker run --rm \
    -v "$REPO_ROOT:/src:ro" \
    -v "$GO_MOD_CACHE:/go/pkg/mod" \
    -v "$GO_BUILD_CACHE:/root/.cache/go-build" \
    -w /src \
    "$GO_IMAGE" \
    bash scripts/test-windows-build.sh
fi

if [[ "$FULL_VERIFICATION" == "true" || "$BROWSER_CHANGED" == "true" ]]; then
  echo
  echo "========================================"
  echo "Browser extension unit tests"
  echo "========================================"

  docker run --rm     -v "$REPO_ROOT:/src:ro"     -w /src/browser-extension/chrome     "$NODE_IMAGE"     node --test

  echo "PASS: browser extension unit tests"

  echo
  echo "========================================"
  echo "Chromium extension smoke test"
  echo "========================================"

  docker run --rm     --shm-size=256m     -v "$REPO_ROOT:/src"     -v "$NPM_CACHE:/root/.npm"     -v "$NODE_MODULES:/src/browser-extension/chrome/node_modules"     -w /src/browser-extension/chrome     "$PLAYWRIGHT_IMAGE"     bash -euc '
      npm install --no-package-lock --no-audit --no-fund
      node --test smoke.e2e.js
    '

  echo "PASS: Chromium extension smoke test"
fi

echo
echo "========================================"
echo "Git diff check"
echo "========================================"

git diff --check "$BASE_REF"...HEAD

echo
echo "========================================"
echo "VERIFICATION PASSED"
echo "========================================"
