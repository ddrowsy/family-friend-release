#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
codeql="$repo_root/release/public-overlay/.github/workflows/codeql.yml"

if [[ ! -f "$codeql" ]]; then
  echo "public CodeQL workflow is missing: $codeql" >&2
  exit 1
fi

for required in \
  'name: CodeQL' \
  '      - main' \
  "    - cron: '23 4 * * 1'" \
  'security-events: write' \
  'runs-on: ubuntu-latest' \
  '- language: go' \
  'build-mode: autobuild' \
  '- language: javascript-typescript' \
  'build-mode: none' \
  'actions/setup-go@v7' \
  'go-version-file: go.mod' \
  'github/codeql-action/init@v4' \
  'github/codeql-action/analyze@v4' \
  'category: "/language:${{ matrix.language }}"'; do
  if ! grep -Fq -- "$required" "$codeql"; then
    echo "public CodeQL workflow is missing required behavior: $required" >&2
    exit 1
  fi
done

for forbidden in \
  'self-hosted' \
  'PUBLIC_RELEASE_TOKEN' \
  'secrets.' \
  'ddrowsy/family-friend' \
  'publish-release' \
  'release_context' \
  "'release/*'"; do
  if grep -Fq -- "$forbidden" "$codeql"; then
    echo "public CodeQL workflow contains forbidden dependency: $forbidden" >&2
    exit 1
  fi
done

echo 'public CodeQL workflow tests passed'
