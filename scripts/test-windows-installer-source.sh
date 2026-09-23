#!/usr/bin/env bash
set -euo pipefail

PACKAGE_FILE="installer/windows/Package.wxs"
TOOL_MANIFEST="installer/windows/.config/dotnet-tools.json"
BUILD_SCRIPT="installer/windows/build.ps1"

expected_files=(
  "drowsyfriend.exe"
  "drowsyfriend-ui.exe"
  "drowsyfriend-session-worker.exe"
)

for file in "${expected_files[@]}"; do
  if ! grep -Fq "$file" "$PACKAGE_FILE"; then
    echo "missing installer file entry: $file" >&2
    exit 1
  fi
done

file_count="$(grep -c '<File ' "$PACKAGE_FILE")"
if [[ "$file_count" -ne "${#expected_files[@]}" ]]; then
  echo "expected ${#expected_files[@]} installer File entries, found $file_count" >&2
  exit 1
fi

if grep -Fq "drowsyfriendctl.exe" "$PACKAGE_FILE"; then
  echo "drowsyfriendctl.exe must not be packaged" >&2
  exit 1
fi

if ! grep -Fq 'Id="ProgramFiles64Folder"' "$PACKAGE_FILE" || ! grep -Fq 'Name="Family Friend"' "$PACKAGE_FILE"; then
  echo "installer must target Program Files\\Family Friend" >&2
  exit 1
fi

for metadata in 'Manufacturer="Family Friend"' 'ProductCode="{' 'UpgradeCode="{'; do
  if ! grep -Fq "$metadata" "$PACKAGE_FILE"; then
    echo "missing stable package metadata: $metadata" >&2
    exit 1
  fi
done

if ! grep -Fq '"version": "6.0.2"' "$TOOL_MANIFEST"; then
  echo "WiX tool version must remain pinned" >&2
  exit 1
fi

if ! grep -Fq 'msi validate' "$BUILD_SCRIPT" || ! grep -Fq 'msi decompile' "$BUILD_SCRIPT"; then
  echo "installer build must validate and inspect the produced MSI" >&2
  exit 1
fi

echo "PASS: Windows installer source"
