#!/usr/bin/env bash
set -euo pipefail

agent_output="$(mktemp "${TMPDIR:-/tmp}/drowsyfriend-windows-XXXXXX.exe")"
worker_output="$(mktemp "${TMPDIR:-/tmp}/drowsyfriend-session-worker-XXXXXX.exe")"
ui_output="$(mktemp "${TMPDIR:-/tmp}/drowsyfriend-ui-XXXXXX.exe")"
bridge_test_output="$(mktemp "${TMPDIR:-/tmp}/sessionbridge-windows-test-XXXXXX.exe")"
trap 'rm -f "$agent_output" "$worker_output" "$ui_output" "$bridge_test_output"' EXIT

GOOS=windows GOARCH=amd64 go build \
  -buildvcs=false \
  -o "$agent_output" \
  ./cmd/drowsyfriend

GOOS=windows GOARCH=amd64 go build \
  -buildvcs=false \
  -ldflags="-H=windowsgui" \
  -o "$worker_output" \
  ./cmd/drowsyfriend-session-worker

GOOS=windows GOARCH=amd64 go build \
  -tags=ci \
  -buildvcs=false \
  -ldflags="-H=windowsgui" \
  -o "$ui_output" \
  ./cmd/drowsyfriend-ui

GOOS=windows GOARCH=amd64 go test \
  -c \
  -buildvcs=false \
  -o "$bridge_test_output" \
  ./internal/sessionbridge

echo "PASS: Windows agent, session worker, desktop UI, and session bridge build"
