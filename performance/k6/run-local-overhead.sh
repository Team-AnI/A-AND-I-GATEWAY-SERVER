#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="${1:-$ROOT_DIR/performance/k6/env.example}"
COMPOSE_FILE="$ROOT_DIR/performance/mock-upstream/docker-compose.performance.yml"
PERFORMANCE_JWT_SECRET="${PERFORMANCE_JWT_SECRET:-performance-local-only-jwt-secret-at-least-32-bytes}"
EXPECTED_K6_VERSION="$(tr -d '[:space:]' < "$ROOT_DIR/performance/k6/K6_VERSION")"

load_env_defaults() {
  local file="$1"
  local line key value
  [[ -f "$file" ]] || return
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -n "$line" ]] || continue
    [[ "$line" == *=* ]] || continue
    key="${line%%=*}"
    value="${line#*=}"
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    if [[ -z "${!key+x}" ]]; then
      export "$key=$value"
    fi
  done < "$file"
}

load_env_defaults "$ENV_FILE"

export BASE_URL="${BASE_URL:-http://localhost:${GATEWAY_PORT:-8080}}"
export UPSTREAM_BASE_URL="${UPSTREAM_BASE_URL:-http://localhost:${MOCK_UPSTREAM_PORT:-18080}}"
export DOWNSTREAM_URL="${DOWNSTREAM_URL:-$UPSTREAM_BASE_URL}"
export GATEWAY_MANAGEMENT_PORT="${GATEWAY_MANAGEMENT_PORT:-9090}"
export GATEWAY_MANAGEMENT_URL="${GATEWAY_MANAGEMENT_URL:-http://localhost:${GATEWAY_MANAGEMENT_PORT}}"
export TARGET_ENVIRONMENT="${TARGET_ENVIRONMENT:-${TARGET_ENV:-local}}"
export TARGET_ENV="${TARGET_ENV:-$TARGET_ENVIRONMENT}"
export ALLOW_REMOTE_LOAD_TEST=false
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
export RUN_ID="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)-overhead}"
export RUN_REPEAT="${RUN_REPEAT:-3}"
export RUN_ORDER="${RUN_ORDER:-alternating}"
export COMMIT_SHA="${COMMIT_SHA:-$(git -C "$ROOT_DIR" rev-parse HEAD)}"
export GIT_DIRTY="${GIT_DIRTY:-$(if git -C "$ROOT_DIR" diff --quiet && git -C "$ROOT_DIR" diff --cached --quiet; then echo false; else echo true; fi)}"
export SKIP_AUTH_SCENARIOS="${SKIP_AUTH_SCENARIOS:-false}"
export EXPECT_REPORT_502="${EXPECT_REPORT_502:-true}"
export AUTH_SERVICE_URI="${AUTH_SERVICE_URI:-http://mock-upstream:18080}"
export POST_SERVICE_URI="${POST_SERVICE_URI:-http://mock-upstream:18080}"
export ONLINE_JUDGE_SERVICE_URI="${ONLINE_JUDGE_SERVICE_URI:-http://mock-upstream:18080}"
export PERFORMANCE_REPORT_SERVICE_URI="${PERFORMANCE_REPORT_SERVICE_URI:-http://mock-upstream:18080}"
export OVERHEAD_PAYLOAD_BYTES="${OVERHEAD_PAYLOAD_BYTES:-1024,65536,1048576}"
export OVERHEAD_ROUTE_KINDS="${OVERHEAD_ROUTE_KINDS:-public,protected}"
export OVERHEAD_LOGGING_MODES="${OVERHEAD_LOGGING_MODES:-enabled,disabled}"
export MEASUREMENT_DATE="${MEASUREMENT_DATE:-$(date -u +%Y-%m-%d)}"
export PERFORMANCE_JWT_SECRET
K6_BIN="${K6_BIN:-k6}"

fail() {
  echo "$1" >&2
  exit 1
}

to_lower() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "Missing required command: $1"
  fi
}

require_executable() {
  local executable="$1"
  if [[ "$executable" == */* ]]; then
    if [[ ! -x "$executable" ]]; then
      fail "Missing executable: $executable"
    fi
    return
  fi
  require_command "$executable"
}

url_host() {
  local url="$1"
  local stripped="${url#*://}"
  stripped="${stripped%%/*}"
  stripped="${stripped%%:*}"
  stripped="${stripped#[}"
  stripped="${stripped%]}"
  echo "$stripped"
}

assert_not_prod_env() {
  local env_lower
  local target_lower
  env_lower="$(to_lower "$TARGET_ENVIRONMENT")"
  target_lower="$(to_lower "$TARGET_ENV")"
  if [[ "$env_lower" == "prod" || "$env_lower" == "production" || "$target_lower" == "prod" || "$target_lower" == "production" ]]; then
    fail "TARGET_ENV/TARGET_ENVIRONMENT prod and production are blocked for local overhead measurements."
  fi
}

assert_no_forbidden_target() {
  local name="$1"
  local url="$2"
  local lower
  local host
  lower="$(to_lower "$url")"
  host="$(url_host "$url")"
  if [[ "$lower" == *"aandiclub.com"* || "$lower" == *"api.aandiclub.com"* ]]; then
    fail "$name=$url is blocked because production host strings are forbidden."
  fi
  if [[ "$host" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ && "$host" != "127.0.0.1" ]]; then
    fail "$name=$url is blocked because public IPv4 targets are forbidden."
  fi
}

assert_base_url() {
  local name="$1"
  local url="$2"
  local host
  assert_no_forbidden_target "$name" "$url"
  host="$(url_host "$url")"
  if [[ "$host" != "localhost" && "$host" != "127.0.0.1" ]]; then
    fail "$name=$url must be localhost or 127.0.0.1."
  fi
}

assert_downstream_url() {
  local name="$1"
  local url="$2"
  local host
  assert_no_forbidden_target "$name" "$url"
  host="$(url_host "$url")"
  if [[ "$host" != "localhost" && "$host" != "127.0.0.1" && "$host" != "mock-upstream" ]]; then
    fail "$name=$url must be localhost, 127.0.0.1, or mock-upstream."
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
    fail "Missing required command: docker compose or docker-compose"
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

resolve_result_dir() {
  if [[ "$RESULT_DIR" = /* ]]; then
    echo "$RESULT_DIR"
  else
    echo "$ROOT_DIR/$RESULT_DIR"
  fi
}

run_k6() {
  "$K6_BIN" run "$ROOT_DIR/$1"
}

assert_file_exists() {
  local file="$1"
  if [[ ! -s "$file" ]]; then
    fail "Expected result file was not created: $file"
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

set_gateway_logging_mode() {
  local mode="$1"
  if [[ "$mode" == "enabled" ]]; then
    export AANDI_LOGGING_REQUEST_RESPONSE_ENABLED=true
  else
    export AANDI_LOGGING_REQUEST_RESPONSE_ENABLED=false
  fi
  compose up -d --no-deps --force-recreate gateway-performance
  wait_for_url "$GATEWAY_MANAGEMENT_URL/actuator/health" 90
}

run_direct_gateway_pair() {
  local script="$1"
  local direct_file_name="$2"
  local gateway_file_name="$3"
  local run_index="$4"
  local pair_index="$5"

  export PAIR_INDEX="$pair_index"
  export RUN_INDEX="$run_index"
  export PAIR_ORDER
  PAIR_ORDER="$(pair_order_for_index "$pair_index")"

  if [[ "$PAIR_ORDER" == "direct-then-gateway" ]]; then
    export OVERHEAD_TARGET=direct
    export MEASURED_POSITION=1
    run_k6 "$script"
    export OVERHEAD_TARGET=gateway
    export MEASURED_POSITION=2
    run_k6 "$script"
  else
    export OVERHEAD_TARGET=gateway
    export MEASURED_POSITION=1
    run_k6 "$script"
    export OVERHEAD_TARGET=direct
    export MEASURED_POSITION=2
    run_k6 "$script"
  fi

  assert_file_exists "$RESULT_DIR_PATH/$direct_file_name-$RUN_ID-r$RUN_INDEX.json"
  assert_file_exists "$RESULT_DIR_PATH/$gateway_file_name-$RUN_ID-r$RUN_INDEX.json"
}

switch_report_downstream_to_connection_failure() {
  export PERFORMANCE_REPORT_SERVICE_URI="http://127.0.0.1:65534"
  compose up -d --no-deps --force-recreate gateway-performance
  wait_for_url "$GATEWAY_MANAGEMENT_URL/actuator/health" 90
}

assert_not_prod_env
assert_base_url BASE_URL "$BASE_URL"
assert_downstream_url UPSTREAM_BASE_URL "$UPSTREAM_BASE_URL"
assert_downstream_url DOWNSTREAM_URL "$DOWNSTREAM_URL"
assert_downstream_url AUTH_SERVICE_URI "$AUTH_SERVICE_URI"
assert_downstream_url POST_SERVICE_URI "$POST_SERVICE_URI"
assert_downstream_url ONLINE_JUDGE_SERVICE_URI "$ONLINE_JUDGE_SERVICE_URI"
assert_downstream_url PERFORMANCE_REPORT_SERVICE_URI "$PERFORMANCE_REPORT_SERVICE_URI"
assert_base_url GATEWAY_MANAGEMENT_URL "$GATEWAY_MANAGEMENT_URL"

require_command docker
require_executable "$K6_BIN"
require_command java
require_command python3
require_command curl
select_compose

K6_VERSION_OUTPUT="$("$K6_BIN" version | head -n1)"
K6_VERSION="$(awk '{print $2}' <<< "$K6_VERSION_OUTPUT")"
if [[ "$K6_VERSION" != "$EXPECTED_K6_VERSION" || "$K6_VERSION_OUTPUT" == *"commit/devel"* ]]; then
  echo "Expected k6 version: $EXPECTED_K6_VERSION" >&2
  echo "Actual k6 version: $K6_VERSION_OUTPUT" >&2
  echo "Install the official k6 $EXPECTED_K6_VERSION release or switch your PATH to that version before running local measurements." >&2
  exit 1
fi
export K6_VERSION

JVM_VERSION="$(java -version 2>&1 | head -n1)"
DOCKER_VERSION="$(docker version --format '{{.Server.Version}}')"
RESULT_DIR_PATH="$(resolve_result_dir)"
mkdir -p "$RESULT_DIR_PATH"

cd "$ROOT_DIR"
compose config >/dev/null
./gradlew bootJar

trap cleanup EXIT
export AANDI_LOGGING_REQUEST_RESPONSE_ENABLED=true
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

IFS=',' read -r -a PAYLOAD_SET <<< "$OVERHEAD_PAYLOAD_BYTES"
for payload in "${PAYLOAD_SET[@]}"; do
  export PAYLOAD_BYTES="$payload"
  for pair_index in $(seq 1 "$RUN_REPEAT"); do
    run_direct_gateway_pair \
      "performance/k6/gateway-payload-overhead.js" \
      "gateway-payload-overhead-direct" \
      "gateway-payload-overhead-gateway" \
      "payload-$payload-$pair_index" \
      "$pair_index"
  done
done

export PAYLOAD_BYTES=1024
IFS=',' read -r -a ROUTE_SET <<< "$OVERHEAD_ROUTE_KINDS"
for route_kind in "${ROUTE_SET[@]}"; do
  export OVERHEAD_ROUTE_KIND="$route_kind"
  for pair_index in $(seq 1 "$RUN_REPEAT"); do
    run_direct_gateway_pair \
      "performance/k6/gateway-route-overhead.js" \
      "gateway-route-overhead-$route_kind-direct" \
      "gateway-route-overhead-$route_kind-gateway" \
      "route-$route_kind-$pair_index" \
      "$pair_index"
  done
done

IFS=',' read -r -a LOGGING_SET <<< "$OVERHEAD_LOGGING_MODES"
for logging_mode in "${LOGGING_SET[@]}"; do
  export OVERHEAD_LOGGING_MODE="$logging_mode"
  set_gateway_logging_mode "$logging_mode"
  for pair_index in $(seq 1 "$RUN_REPEAT"); do
    run_direct_gateway_pair \
      "performance/k6/gateway-logging-overhead.js" \
      "gateway-logging-overhead-$logging_mode-direct" \
      "gateway-logging-overhead-$logging_mode-gateway" \
      "logging-$logging_mode-$pair_index" \
      "$pair_index"
  done
done

set_gateway_logging_mode enabled
if [[ "$EXPECT_REPORT_502" == "true" ]]; then
  switch_report_downstream_to_connection_failure
fi

export RUN_INDEX=contract
run_k6 "performance/k6/gateway-error-contract.js"
assert_file_exists "$RESULT_DIR_PATH/gateway-error-contract-$RUN_ID-rcontract.json"

export RUN_INDEX=rate-limit
run_k6 "performance/k6/gateway-rate-limit.js"
assert_file_exists "$RESULT_DIR_PATH/gateway-rate-limit-$RUN_ID-rrate-limit.json"

python3 "$ROOT_DIR/performance/aggregate/summarize_gateway_overhead.py" \
  --input "$RESULT_DIR_PATH" \
  --run-prefix "$RUN_ID" \
  --out-json "$RESULT_DIR_PATH/$MEASUREMENT_DATE-gateway-local-overhead.json" \
  --out-md "$RESULT_DIR_PATH/$MEASUREMENT_DATE-gateway-local-overhead.md" \
  --docs-json "$ROOT_DIR/docs/performance/data/$MEASUREMENT_DATE-gateway-local-overhead.json" \
  --measurement-date "$MEASUREMENT_DATE" \
  --commit-sha "$COMMIT_SHA" \
  --k6-version "$K6_VERSION" \
  --jvm-version "$JVM_VERSION" \
  --docker-version "$DOCKER_VERSION" \
  --planned-vus "$LOAD_VUS" \
  --planned-duration "$TEST_DURATION" \
  --planned-payload-bytes "$OVERHEAD_PAYLOAD_BYTES" \
  --planned-base-url "$BASE_URL" \
  --planned-upstream-url "$UPSTREAM_BASE_URL" \
  --planned-downstream-url "$DOWNSTREAM_URL"
