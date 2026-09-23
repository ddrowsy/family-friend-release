#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

mock_data=false
case "${1:-}" in
  "")
    ;;
  --mock-data)
    mock_data=true
    ;;
  *)
    echo "usage: $0 [--mock-data]" >&2
    exit 2
    ;;
esac

output_dir="dist"
if [[ "$mock_data" == true ]]; then
  output_dir="dist/mock"
fi

mkdir -p "$output_dir"
GOOS=windows GOARCH="${GOARCH:-amd64}" CGO_ENABLED=0 \
  go build -buildvcs=false -o "$output_dir/drowsyfriend.exe" ./cmd/drowsyfriend

if [[ "$mock_data" == true ]]; then
  cat > "$output_dir/run-mock-agent.bat" <<'BAT'
@echo off
"%~dp0drowsyfriend.exe" --mock-data
BAT
fi
