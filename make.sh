#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/tmp/drowsyfriend-go-build}"

CONFIG_SOURCE="configs/default.policy.json"

config_schema_version() {
	local file="$1"
	sed -n 's/^[[:space:]]*"schema_version"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$file" | head -n 1
}

sync_config() {
	local output_dir="$1"
	local target_dir="$output_dir/configs"
	local target="$target_dir/default.policy.json"
	local source_version
	local target_version

	source_version="$(config_schema_version "$CONFIG_SOURCE")"
	if [[ -z "$source_version" ]]; then
		echo "missing schema_version in $CONFIG_SOURCE" >&2
		exit 1
	fi

	target_version=""
	if [[ -f "$target" ]]; then
		target_version="$(config_schema_version "$target")"
	fi

	if [[ ! -f "$target" || "$target_version" != "$source_version" ]]; then
		mkdir -p "$target_dir"
		cp "$CONFIG_SOURCE" "$target"
		echo "copied $CONFIG_SOURCE to $target (schema_version=$source_version)"
	fi
}

usage() {
	cat <<'EOF'
Usage: ./make.sh <command>

Commands:
  test              Run go test ./...
  run               Run drowsyfriend
  ctl-validate      Validate configs/default.policy.json
  ctl-modules       List available modules
  build             Build current-platform binaries into bin/
  build-windows     Cross-build Windows .exe binaries into bin/windows/
  clean             Remove build outputs
EOF
}

cmd="${1:-}"

case "$cmd" in
	test)
		go test ./...
		;;
	run)
		go run ./cmd/drowsyfriend
		;;
	ctl-validate)
		go run ./cmd/drowsyfriendctl policy validate
		;;
	ctl-modules)
		go run ./cmd/drowsyfriendctl modules list
		;;
	build)
		mkdir -p bin
		go build -buildvcs=false -o bin/drowsyfriend ./cmd/drowsyfriend
		go build -buildvcs=false -o bin/drowsyfriendctl ./cmd/drowsyfriendctl
		sync_config "bin"
		;;
	build-windows)
		mkdir -p bin/windows
		GOOS=windows GOARCH=amd64 go build -buildvcs=false -o bin/windows/drowsyfriend.exe ./cmd/drowsyfriend
		GOOS=windows GOARCH=amd64 go build -buildvcs=false -o bin/windows/drowsyfriendctl.exe ./cmd/drowsyfriendctl
		GOOS=windows GOARCH=amd64 go build \
			-buildvcs=false \
			-ldflags="-H=windowsgui" \
			-o bin/windows/drowsyfriend-session-worker.exe \
			./cmd/drowsyfriend-session-worker
		sync_config "bin/windows"
		;;
	clean)
		rm -rf bin
		;;
	""|-h|--help|help)
		usage
		;;
	*)
		usage >&2
		exit 2
		;;
esac
