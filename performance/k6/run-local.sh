#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="${1:-$ROOT_DIR/performance/k6/env.example}"
COMPOSE_FILE="$ROOT_DIR/performance/mock-upstream/docker-compose.performance.yml"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

export BASE_URL="${BASE_URL:-http://localhost:8080}"
export UPSTREAM_BASE_URL="${UPSTREAM_BASE_URL:-http://localhost:18080}"
export ALLOW_REMOTE_LOAD_TEST="${ALLOW_REMOTE_LOAD_TEST:-false}"
export LOAD_VUS="${LOAD_VUS:-1}"
export TEST_DURATION="${TEST_DURATION:-10s}"
export RESULT_DIR="${RESULT_DIR:-performance/results}"
export PUBLIC_ROUTE_PATH="${PUBLIC_ROUTE_PATH:-/v2/blogs}"
export PROTECTED_ROUTE_PATH="${PROTECTED_ROUTE_PATH:-/v1/me}"
export FORBIDDEN_ROUTE_PATH="${FORBIDDEN_ROUTE_PATH:-/v1/admin/ping}"
export RATE_LIMIT_PATH="${RATE_LIMIT_PATH:-/v1/auth/login}"
export PAYLOAD_BYTES="${PAYLOAD_BYTES:-1024}"
export MOCK_DELAY_MS="${MOCK_DELAY_MS:-0}"
export MOCK_STATUS="${MOCK_STATUS:-200}"
export AUTH_LOGIN_RATE_LIMIT_PER_MINUTE="${AUTH_LOGIN_RATE_LIMIT_PER_MINUTE:-10}"
export RUN_ID="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
export RUN_REPEAT="${RUN_REPEAT:-1}"
export RUN_ORDER="${RUN_ORDER:-direct-then-gateway}"
export COMMIT_SHA="${COMMIT_SHA:-$(git -C "$ROOT_DIR" rev-parse HEAD)}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

wait_for_url() {
  local url="$1"
  local attempts="${2:-60}"
  local i
  for i in $(seq 1 "$attempts"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "Timed out waiting for $url" >&2
  exit 1
}

require_command docker
require_command k6
require_command python3
require_command curl

if docker compose version >/dev/null 2>&1; then
  COMPOSE_CMD=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_CMD=(docker-compose)
else
  echo "Missing required command: docker compose or docker-compose" >&2
  exit 1
fi

mkdir -p "$ROOT_DIR/$RESULT_DIR"

cd "$ROOT_DIR"
./gradlew bootJar

"${COMPOSE_CMD[@]}" -f "$COMPOSE_FILE" up -d --build
wait_for_url "$UPSTREAM_BASE_URL/health" 60
wait_for_url "http://localhost:9090/actuator/health" 90

for run_index in $(seq 1 "$RUN_REPEAT"); do
  export RUN_INDEX="$run_index"

  k6 run "$ROOT_DIR/performance/k6/direct-upstream.js"
  k6 run "$ROOT_DIR/performance/k6/gateway-public-route.js"

  python3 "$ROOT_DIR/performance/compare/compare_results.py" \
    --direct "$ROOT_DIR/$RESULT_DIR/direct-upstream-$RUN_ID-r$RUN_INDEX.json" \
    --gateway "$ROOT_DIR/$RESULT_DIR/gateway-public-route-$RUN_ID-r$RUN_INDEX.json" \
    --output-dir "$ROOT_DIR/$RESULT_DIR"
done

export RUN_INDEX=protected
k6 run "$ROOT_DIR/performance/k6/gateway-protected-route.js"

export RUN_INDEX=contract
k6 run "$ROOT_DIR/performance/k6/gateway-error-contract.js"

export RUN_INDEX=rate-limit
k6 run "$ROOT_DIR/performance/k6/gateway-rate-limit.js"
