#!/usr/bin/env bash
set -euo pipefail

TARGET_URL="${1:-http://127.0.0.1:8080/}"
CONNECTIONS="${2:-10000}"
DURATION="${3:-60}"

# Increase ulimit before running very high connection counts.
ulimit -n 200000 || true

autocannon \
  --connections "${CONNECTIONS}" \
  --pipelining 1 \
  --duration "${DURATION}" \
  --renderStatusCodes \
  "${TARGET_URL}"
