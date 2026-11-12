#!/usr/bin/env bash
# Builds PhoneLock.exe for 64-bit Windows. Run from anywhere.
set -euo pipefail
cd "$(dirname "$0")"
KEY="$(tr -d '[:space:]' < ../shared/key.txt)"
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.secretKey=${KEY}" \
  -o dist/PhoneLock.exe .
echo "Built pc/dist/PhoneLock.exe"
