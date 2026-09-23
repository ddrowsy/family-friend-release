#!/usr/bin/env bash
# update-context.sh — Change detector for project.mcp.json manifest.
#
# Usage: ./bin/update-context.sh
#
# Output (stdout):
#   changed: file1, file2
#   new: file3
#   deleted: file4
#   old_manifest_md5: <md5>
#   new_manifest_json: { ... }
#
# Exit code: 0 (always, even if no files found)
#
# The AI reads this output, reads changed/new files, edits project.mcp.json,
# then replaces the entire "manifest" block and "manifest_md5" field.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
MCP_FILE="$PROJECT_DIR/project.mcp.json"

# ── tracked files (non-test Go + config/metadata) ──────────────────────────
tracked_files=(
  cmd/drowsyfriend/main.go
  cmd/drowsyfriendctl/main.go
  configs/default.policy.json
  go.mod
  internal/ai/classifier.go
  internal/ai/mock.go
  internal/app/bootstrap.go
  internal/app/runtime.go
  internal/audit/event.go
  internal/audit/logger.go
  internal/config/loader.go
  internal/config/policy.go
  internal/config/validator.go
  internal/modules/kidcontrol/action/action.go
  internal/modules/kidcontrol/action/close_app.go
  internal/modules/kidcontrol/action/log_action.go
  internal/modules/kidcontrol/config.go
  internal/modules/kidcontrol/decision/evaluator.go
  internal/modules/kidcontrol/module.go
  internal/modules/kidcontrol/monitor/app_monitor.go
  internal/modules/kidcontrol/monitor/screen_monitor.go
  internal/modules/kidcontrol/monitor/web_monitor.go
  internal/modules/kidcontrol/monitor/window_monitor.go
  internal/modules/kidcontrol/service.go
  internal/modules/kidcontrol/webmonitor.go
  internal/modules/module.go
  internal/modules/registry.go
  internal/platform/platform.go
  internal/platform/windows/close_stub.go
  internal/platform/windows/close_windows.go
  internal/platform/windows/platform.go
  internal/platform/windows/process_stub.go
  internal/platform/windows/process_windows.go
  internal/platform/windows/screen.go
  internal/platform/windows/window_stub.go
  internal/platform/windows/window_windows.go
  make.sh
  schemas/policy.schema.json
)

# ── compute MD5 for each file ──────────────────────────────────────────────
declare -A current_md5s
for f in "${tracked_files[@]}"; do
  full="$PROJECT_DIR/$f"
  if [ -f "$full" ]; then
    current_md5s["$f"]=$(md5sum "$full" | awk '{print $1}')
  fi
done

# ── read old manifest from MCP file ────────────────────────────────────────
old_manifest_md5=""
declare -A old_md5s

if [ -f "$MCP_FILE" ]; then
  # Extract manifest_md5 value
  old_manifest_md5=$(python3 -c "
import json, sys
try:
    d = json.load(open('$MCP_FILE'))
    print(d.get('manifest_md5', ''))
except: print('')
" 2>/dev/null || echo "")

  # Extract per-file MD5s from manifest section
  mapfile -t manifest_entries < <(python3 -c "
import json, sys
try:
    d = json.load(open('$MCP_FILE'))
    manifest = d.get('manifest', {})
    files = manifest.get('files', {})
    for path, md5 in sorted(files.items()):
        print(f'{path}\t{md5}')
except:
    pass
" 2>/dev/null || true)

  for entry in "${manifest_entries[@]}"; do
    [ -z "$entry" ] && continue
    f=$(echo "$entry" | cut -f1)
    m=$(echo "$entry" | cut -f2)
    old_md5s["$f"]="$m"
  done
fi

# ── classify changes ───────────────────────────────────────────────────────
declare -a changed_files=()
declare -a new_files=()
declare -a deleted_files=()

# Check each current file
for f in "${!current_md5s[@]}"; do
  old="${old_md5s[$f]:-}"
  new="${current_md5s[$f]}"
  if [ -z "$old" ]; then
    new_files+=("$f")
  elif [ "$old" != "$new" ]; then
    changed_files+=("$f")
  fi
done

# Check for deleted files
for f in "${!old_md5s[@]}"; do
  if [ -z "${current_md5s[$f]:-}" ]; then
    deleted_files+=("$f")
  fi
done

# ── sort arrays ────────────────────────────────────────────────────────────
IFS=$'\n'
changed_sorted=($(sort <<<"${changed_files[*]}" 2>/dev/null || true))
new_sorted=($(sort <<<"${new_files[*]}" 2>/dev/null || true))
deleted_sorted=($(sort <<<"${deleted_files[*]}" 2>/dev/null || true))
unset IFS

# ── build new manifest JSON ───────────────────────────────────────────────
now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
new_manifest_json=$(python3 -c "
import json, sys

files = {}
entries = '''$(
  for f in "${!current_md5s[@]}"; do
    printf '%s\t%s\n' "$f" "${current_md5s[$f]}"
  done
)'''

for line in entries.strip().split('\n'):
    if not line.strip():
        continue
    parts = line.split('\t')
    if len(parts) == 2:
        files[parts[0]] = parts[1]

manifest = {'updated': '$now', 'files': dict(sorted(files.items()))}
print(json.dumps(manifest))
")

new_manifest_md5=$(echo "$new_manifest_json" | md5sum | awk '{print $1}')

# ── output ─────────────────────────────────────────────────────────────────
if [ ${#changed_sorted[@]} -gt 0 ] && [ -n "${changed_sorted[0]:-}" ]; then
  printf 'changed: %s\n' "$(IFS=', '; echo "${changed_sorted[*]}")"
fi
if [ ${#new_sorted[@]} -gt 0 ] && [ -n "${new_sorted[0]:-}" ]; then
  printf 'new: %s\n' "$(IFS=', '; echo "${new_sorted[*]}")"
fi
if [ ${#deleted_sorted[@]} -gt 0 ] && [ -n "${deleted_sorted[0]:-}" ]; then
  printf 'deleted: %s\n' "$(IFS=', '; echo "${deleted_sorted[*]}")"
fi

printf 'old_manifest_md5: %s\n' "$old_manifest_md5"
printf 'new_manifest_json: %s\n' "$new_manifest_json"
