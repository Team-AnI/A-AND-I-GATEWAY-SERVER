#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="${1:-$ROOT_DIR/performance/k6/env.example}"
COMPOSE_FILE="$ROOT_DIR/performance/mock-upstream/docker-compose.performance.yml"
PERFORMANCE_JWT_SECRET="${PERFORMANCE_JWT_SECRET:-performance-local-only-jwt-secret-at-least-32-bytes}"
EXPECTED_K6_VERSION="$(tr -d '[:space:]' < "$ROOT_DIR/performance/k6/K6_VERSION")"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

export BASE_URL="${BASE_URL:-http://localhost:${GATEWAY_PORT:-8080}}"
export UPSTREAM_BASE_URL="${UPSTREAM_BASE_URL:-http://localhost:${MOCK_UPSTREAM_PORT:-18080}}"
export GATEWAY_MANAGEMENT_PORT="${GATEWAY_MANAGEMENT_PORT:-9090}"
export GATEWAY_MANAGEMENT_URL="${GATEWAY_MANAGEMENT_URL:-http://localhost:${GATEWAY_MANAGEMENT_PORT}}"
export ALLOW_REMOTE_LOAD_TEST="${ALLOW_REMOTE_LOAD_TEST:-false}"
export TARGET_ENVIRONMENT="${TARGET_ENVIRONMENT:-local}"
export REMOTE_TARGET_ALLOWLIST="${REMOTE_TARGET_ALLOWLIST:-}"
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
export AUTH_ISSUER_URI="${AUTH_ISSUER_URI:-http://localhost:9000}"
export AUTH_AUDIENCE="${AUTH_AUDIENCE:-aandi-gateway}"
export AUTH_LOGIN_RATE_LIMIT_PER_MINUTE="${AUTH_LOGIN_RATE_LIMIT_PER_MINUTE:-10}"
export RUN_ID="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
export RUN_REPEAT="${RUN_REPEAT:-1}"
export RUN_ORDER="${RUN_ORDER:-alternating}"
export COMMIT_SHA="${COMMIT_SHA:-$(git -C "$ROOT_DIR" rev-parse HEAD)}"
export GIT_DIRTY="${GIT_DIRTY:-$(if git -C "$ROOT_DIR" diff --quiet && git -C "$ROOT_DIR" diff --cached --quiet; then echo false; else echo true; fi)}"
export SKIP_AUTH_SCENARIOS="${SKIP_AUTH_SCENARIOS:-false}"
export EXPECT_REPORT_502="${EXPECT_REPORT_502:-true}"
export PERFORMANCE_REPORT_SERVICE_URI="${PERFORMANCE_REPORT_SERVICE_URI:-http://mock-upstream:18080}"
export PERFORMANCE_JWT_SECRET

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
  return 1
}

select_compose() {
  if docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD=(docker compose)
  elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD=(docker-compose)
  else
    echo "Missing required command: docker compose or docker-compose" >&2
    exit 1
  fi
}

compose() {
  "${COMPOSE_CMD[@]}" -f "$COMPOSE_FILE" "$@"
}

dump_compose_debug() {
  echo "Docker Compose status:" >&2
  compose ps >&2 || true
  echo "Docker Compose logs:" >&2
  compose logs --tail=200 mock-upstream gateway-performance redis-performance >&2 || true
}

cleanup() {
  local exit_code=$?
  if [[ "$exit_code" -ne 0 ]]; then
    dump_compose_debug
  fi
  if [[ "${KEEP_PERFORMANCE_STACK:-false}" != "true" ]]; then
    compose down --remove-orphans >/dev/null 2>&1 || true
  fi
  exit "$exit_code"
}

run_k6() {
  k6 run "$ROOT_DIR/$1"
}

assert_file_exists() {
  local file="$1"
  if [[ ! -s "$file" ]]; then
    echo "Expected result file was not created: $file" >&2
    exit 1
  fi
}

resolve_result_dir() {
  if [[ "$RESULT_DIR" = /* ]]; then
    echo "$RESULT_DIR"
  else
    echo "$ROOT_DIR/$RESULT_DIR"
  fi
}

generate_tokens() {
  if [[ "$SKIP_AUTH_SCENARIOS" == "true" ]]; then
    return
  fi
  USER_ACCESS_TOKEN="$(python3 "$ROOT_DIR/performance/k6/tools/generate_test_jwt.py" \
    --secret "$PERFORMANCE_JWT_SECRET" \
    --issuer "$AUTH_ISSUER_URI" \
    --audience "$AUTH_AUDIENCE" \
    --role USER)"
  ADMIN_ACCESS_TOKEN="$(python3 "$ROOT_DIR/performance/k6/tools/generate_test_jwt.py" \
    --secret "$PERFORMANCE_JWT_SECRET" \
    --issuer "$AUTH_ISSUER_URI" \
    --audience "$AUTH_AUDIENCE" \
    --role ADMIN)"
  export USER_ACCESS_TOKEN ADMIN_ACCESS_TOKEN
}

pair_order_for_index() {
  local index="$1"
  if (( index % 2 == 1 )); then
    echo "direct-then-gateway"
  else
    echo "gateway-then-direct"
  fi
}

switch_report_downstream_to_connection_failure() {
  export PERFORMANCE_REPORT_SERVICE_URI="http://127.0.0.1:65534"
  compose up -d --no-deps --force-recreate gateway-performance
  wait_for_url "$GATEWAY_MANAGEMENT_URL/actuator/health" 90
}

require_command docker
require_command k6
require_command python3
require_command curl
select_compose

K6_VERSION_OUTPUT="$(k6 version | head -n1)"
K6_VERSION="$(awk '{print $2}' <<< "$K6_VERSION_OUTPUT")"
if [[ "$K6_VERSION" != "$EXPECTED_K6_VERSION" || "$K6_VERSION_OUTPUT" == *"commit/devel"* ]]; then
  echo "Expected k6 version: $EXPECTED_K6_VERSION" >&2
  echo "Actual k6 version: $K6_VERSION_OUTPUT" >&2
  echo "Install the official k6 $EXPECTED_K6_VERSION release or switch your PATH to that version before running local measurements." >&2
  exit 1
fi
export K6_VERSION

RESULT_DIR_PATH="$(resolve_result_dir)"
mkdir -p "$RESULT_DIR_PATH"

cd "$ROOT_DIR"
compose config >/dev/null
./gradlew bootJar

trap cleanup EXIT
compose up -d --build
wait_for_url "$UPSTREAM_BASE_URL/health" 60
wait_for_url "$GATEWAY_MANAGEMENT_URL/actuator/health" 90
generate_tokens

export WARMUP_COMPLETED=false
export RUN_INDEX=warmup
export PAIR_INDEX=1
export MEASURED_POSITION=0
run_k6 "performance/k6/warmup.js"
export WARMUP_COMPLETED=true

for pair_index in $(seq 1 "$RUN_REPEAT"); do
  export PAIR_INDEX="$pair_index"
  export RUN_INDEX="$pair_index"
  export PAIR_ORDER="$(pair_order_for_index "$pair_index")"

  if [[ "$PAIR_ORDER" == "direct-then-gateway" ]]; then
    export MEASURED_POSITION=1
    run_k6 "performance/k6/direct-upstream.js"
    export MEASURED_POSITION=2
    run_k6 "performance/k6/gateway-public-route.js"
  else
    export MEASURED_POSITION=1
    run_k6 "performance/k6/gateway-public-route.js"
    export MEASURED_POSITION=2
    run_k6 "performance/k6/direct-upstream.js"
  fi

  direct_result="$RESULT_DIR_PATH/direct-upstream-$RUN_ID-r$RUN_INDEX.json"
  gateway_result="$RESULT_DIR_PATH/gateway-public-route-$RUN_ID-r$RUN_INDEX.json"
  assert_file_exists "$direct_result"
  assert_file_exists "$gateway_result"
  python3 "$ROOT_DIR/performance/compare/compare_results.py" \
    --direct "$direct_result" \
    --gateway "$gateway_result" \
    --output-dir "$RESULT_DIR_PATH"
done

export RUN_INDEX=protected
export PAIR_INDEX=1
export MEASURED_POSITION=0
run_k6 "performance/k6/gateway-protected-route.js"
assert_file_exists "$RESULT_DIR_PATH/gateway-protected-route-$RUN_ID-rprotected.json"

if [[ "$EXPECT_REPORT_502" == "true" ]]; then
  switch_report_downstream_to_connection_failure
fi

export RUN_INDEX=contract
run_k6 "performance/k6/gateway-error-contract.js"
assert_file_exists "$RESULT_DIR_PATH/gateway-error-contract-$RUN_ID-rcontract.json"

export RUN_INDEX=rate-limit
run_k6 "performance/k6/gateway-rate-limit.js"
assert_file_exists "$RESULT_DIR_PATH/gateway-rate-limit-$RUN_ID-rrate-limit.json"
