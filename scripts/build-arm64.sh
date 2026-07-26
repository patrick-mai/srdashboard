#!/usr/bin/env bash
set -euo pipefail
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o srdashboard-arm64 .
echo "arm64 build OK"
