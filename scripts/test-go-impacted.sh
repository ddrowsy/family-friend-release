#!/usr/bin/env bash
set -euo pipefail

BASE_REF="${1:-origin/main}"
MODE_ONLY=false
if [[ "${2:-}" == "--mode-only" ]]; then
  MODE_ONLY=true
fi

finish() {
  local mode="$1"
  shift

  if "$MODE_ONLY"; then
    printf '%s\n' "$mode"
    exit 0
  fi

  case "$mode" in
    none)
      echo "No Go changes; skipping Go tests."
      ;;
    full)
      echo "Running full Go suite."
      exec go test ./...
      ;;
    impacted)
      echo "Running impacted Go tests:"
      printf '  %s\n' "$@"
      go test "$@"
      ;;
    *)
      echo "Unknown Go test mode: $mode" >&2
      exit 1
      ;;
  esac
}

mapfile -t changed_files < <(git diff --name-only "$BASE_REF"...HEAD)

if printf '%s\n' "${changed_files[@]}" | grep -Eq '^(go\.mod|go\.sum)$'; then
  finish full
fi

go_dirs=()
for path in "${changed_files[@]}"; do
  [[ "$path" == *.go ]] || continue

  dir="$(dirname "$path")"
  if [[ ! -d "$dir" ]]; then
    finish full
  fi
  go_dirs+=("$dir")
done

if [[ "${#go_dirs[@]}" -eq 0 ]]; then
  finish none
fi

declare -A repo_packages=()
declare -A package_deps=()
while IFS='|' read -r package imports test_imports xtest_imports; do
  repo_packages["$package"]=1
  package_deps["$package"]="$imports $test_imports $xtest_imports"
done < <(
  go list -f '{{.ImportPath}}|{{join .Imports " "}}|{{join .TestImports " "}}|{{join .XTestImports " "}}' ./...
)

declare -A impacted=()
declare -A seen_dirs=()
for dir in "${go_dirs[@]}"; do
  [[ -n "${seen_dirs[$dir]:-}" ]] && continue
  seen_dirs["$dir"]=1

  package="$(go list -f '{{.ImportPath}}' "./$dir" 2>/dev/null)" || finish full
  impacted["$package"]=1
done

changed=true
while "$changed"; do
  changed=false
  for package in "${!repo_packages[@]}"; do
    [[ -n "${impacted[$package]:-}" ]] && continue

    for dependency in ${package_deps[$package]}; do
      if [[ -n "${impacted[$dependency]:-}" ]]; then
        impacted["$package"]=1
        changed=true
        break
      fi
    done
  done
done

mapfile -t packages < <(printf '%s\n' "${!impacted[@]}" | sort)
if [[ "${#packages[@]}" -eq 0 ]]; then
  finish full
fi

if [[ "${#packages[@]}" -ge 5 ]]; then
  finish full
fi

finish impacted "${packages[@]}"
