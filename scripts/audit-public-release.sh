#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPORT_PATH="${1:-}"

cd "$REPO_ROOT"
git config --global --add safe.directory "$REPO_ROOT"

report_tmp="$(mktemp)"
module_tmp="$(mktemp -d)"
trap 'rm -f "$report_tmp"; rm -rf "$module_tmp"' EXIT

find_license_file() {
  local module_dir="$1"
  local file

  file="$(
    find "$module_dir" -maxdepth 1 -type f \
      \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) \
      -print \
      | sort \
      | head -n 1
  )"
  if [[ -n "$file" ]]; then
    printf '%s\n' "$file"
    return
  fi

  find "$module_dir" -maxdepth 2 -type f \
    \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) \
    -print \
    | sort \
    | head -n 1
}

classify_license() {
  local file="$1"

  if grep -Fqi 'your choice of exactly one of' "$file" \
    && grep -Fqi 'The FreeType License' "$file" \
    && grep -Eqi 'GNU General Public License' "$file"; then
    printf 'FTL-or-GPL-2.0+|permissive\n'
    return
  fi
  if grep -Eqi 'GNU Affero General Public License' "$file"; then
    printf 'AGPL|restricted\n'
    return
  fi
  if grep -Eqi 'GNU Lesser General Public License' "$file"; then
    printf 'LGPL|reciprocal\n'
    return
  fi
  if grep -Eqi 'GNU General Public License' "$file"; then
    printf 'GPL|restricted\n'
    return
  fi
  if grep -Eqi 'Mozilla Public License' "$file"; then
    printf 'MPL|reciprocal\n'
    return
  fi
  if grep -Eqi 'Apache License' "$file" && grep -Eqi 'Version 2\.0' "$file"; then
    printf 'Apache-2.0|permissive\n'
    return
  fi
  if grep -Fqi 'Permission is hereby granted, free of charge' "$file"; then
    printf 'MIT|permissive\n'
    return
  fi
  if grep -Fqi 'Redistribution and use in source and binary forms' "$file"; then
    if grep -Eqi 'Neither the name|Neither the names' "$file"; then
      printf 'BSD-3-Clause|permissive\n'
    else
      printf 'BSD-2-Clause|permissive\n'
    fi
    return
  fi
  if grep -Fqi 'Permission to use, copy, modify, and/or distribute this software' "$file" \
    && grep -Fqi 'with or without fee' "$file"; then
    printf 'ISC|permissive\n'
    return
  fi
  if grep -Fqi "This software is provided 'as-is'" "$file" \
    && grep -Fqi 'Permission is granted to anyone to use this software for any purpose' "$file"; then
    printf 'Zlib|permissive\n'
    return
  fi
  if grep -Eqi 'public domain|The Unlicense' "$file"; then
    printf 'Public-Domain|unencumbered\n'
    return
  fi

  printf 'UNKNOWN|unknown\n'
}

failures=0

printf 'module,version,license,class,license_file\n' > "$report_tmp"

cp go.mod go.sum "$module_tmp/"
cd "$module_tmp"
go mod download all

while IFS='|' read -r module version module_dir; do
  [[ -n "$module" ]] || continue

  if [[ -z "$module_dir" || ! -d "$module_dir" ]]; then
    printf '%s,%s,UNKNOWN,unknown,missing-module-directory\n' "$module" "$version" >> "$report_tmp"
    failures=$((failures + 1))
    continue
  fi

  license_file="$(find_license_file "$module_dir")"
  if [[ -z "$license_file" ]]; then
    printf '%s,%s,UNKNOWN,unknown,missing-license-file\n' "$module" "$version" >> "$report_tmp"
    failures=$((failures + 1))
    continue
  fi

  classification="$(classify_license "$license_file")"
  license_name="${classification%%|*}"
  license_class="${classification##*|}"
  license_basename="${license_file#"$module_dir"/}"

  printf '%s,%s,%s,%s,%s\n' \
    "$module" \
    "$version" \
    "$license_name" \
    "$license_class" \
    "$license_basename" \
    >> "$report_tmp"

  case "$license_class" in
    permissive|unencumbered)
      ;;
    *)
      failures=$((failures + 1))
      ;;
  esac
done < <(
  go list -m \
    -f '{{if not .Main}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}' \
    all
)

cd "$REPO_ROOT"

{
  head -n 1 "$report_tmp"
  tail -n +2 "$report_tmp" | sort
} > "${report_tmp}.sorted"
mv "${report_tmp}.sorted" "$report_tmp"

cat "$report_tmp"

if [[ -n "$REPORT_PATH" ]]; then
  mkdir -p "$(dirname "$REPORT_PATH")"
  cp "$report_tmp" "$REPORT_PATH"
fi

tracked_third_party="$(
  git ls-files \
    | grep -E '(^|/)(vendor|node_modules|third_party|third-party)/' \
    || true
)"
if [[ -n "$tracked_third_party" ]]; then
  echo >&2
  echo 'ERROR: tracked vendored/third-party dependency directories require manual provenance review:' >&2
  echo "$tracked_third_party" >&2
  failures=$((failures + 1))
fi

node_manifest='browser-extension/chrome/package.json'
if [[ -f "$node_manifest" ]] && grep -Eq '"dependencies"[[:space:]]*:' "$node_manifest"; then
  echo >&2
  echo "ERROR: $node_manifest contains production Node dependencies; add an explicit license audit for them." >&2
  failures=$((failures + 1))
fi

tracked_binary_assets="$(
  git ls-files \
    | grep -Ei '\.(dll|exe|so|dylib|jar|zip|woff2?|ttf|otf)$' \
    || true
)"
if [[ -n "$tracked_binary_assets" ]]; then
  echo >&2
  echo 'ERROR: tracked binary/font assets require explicit provenance review:' >&2
  echo "$tracked_binary_assets" >&2
  failures=$((failures + 1))
fi

provenance_markers="$(
  git grep -nEi \
    '(SPDX-License-Identifier|Copyright \(c\)|Copyright [0-9]{4}|copied from|adapted from|derived from)' \
    -- \
    ':!docs/public-release-license-audit.md' \
    ':!scripts/audit-public-release.sh' \
    || true
)"
if [[ -n "$provenance_markers" ]]; then
  echo >&2
  echo 'NOTICE: source provenance markers found; review before publishing:' >&2
  echo "$provenance_markers" >&2
fi

generated_markers="$(
  git grep -nE 'Code generated .*DO NOT EDIT\.' -- '*.go' \
    || true
)"
if [[ -n "$generated_markers" ]]; then
  echo >&2
  echo 'NOTICE: generated Go source found; confirm generator/source terms before publishing:' >&2
  echo "$generated_markers" >&2
fi

if (( failures > 0 )); then
  echo >&2
  echo "FAIL: public-release license/provenance audit found $failures blocking item(s)." >&2
  exit 1
fi

echo >&2
echo 'PASS: no automatically detected public-release license/provenance blockers.' >&2
