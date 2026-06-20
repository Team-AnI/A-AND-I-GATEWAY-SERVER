export function trimTrailingSlash(value) {
  return value.replace(/\/+$/, '');
}

export function normalizePath(value) {
  if (!value || value === '/') {
    return '/';
  }
  return value.startsWith('/') ? value : `/${value}`;
}

function parseInteger(name, fallback, min, max) {
  const raw = __ENV[name];
  if (raw === undefined || raw === '') {
    return fallback;
  }
  if (!/^\d+$/.test(raw)) {
    throw new Error(`${name} must be an integer`);
  }
  const parsed = Number(raw);
  if (!Number.isSafeInteger(parsed) || parsed < min || parsed > max) {
    throw new Error(`${name} must be between ${min} and ${max}`);
  }
  return parsed;
}

function parsePositiveInt(name, fallback) {
  return parseInteger(name, fallback, 1, Number.MAX_SAFE_INTEGER);
}

function parsePositiveIntValue(name, raw, fallback) {
  if (raw === undefined || raw === '') {
    return fallback;
  }
  if (!/^\d+$/.test(raw)) {
    throw new Error(`${name} must be an integer`);
  }
  const parsed = Number(raw);
  if (!Number.isSafeInteger(parsed) || parsed <= 0) {
    throw new Error(`${name} must be a positive integer`);
  }
  return parsed;
}

function parseNonNegativeInt(name, fallback) {
  return parseInteger(name, fallback, 0, Number.MAX_SAFE_INTEGER);
}

function parseOptionalPositiveInt(name) {
  const raw = __ENV[name];
  if (raw === undefined || raw === '') {
    return null;
  }
  if (!/^\d+$/.test(raw)) {
    throw new Error(`${name} must be an integer when supplied`);
  }
  const parsed = Number(raw);
  if (!Number.isSafeInteger(parsed) || parsed <= 0) {
    throw new Error(`${name} must be a positive integer when supplied`);
  }
  return parsed;
}

function parseBoolean(name, fallback) {
  const raw = __ENV[name];
  if (raw === undefined || raw === '') {
    return fallback;
  }
  return ['1', 'true', 'yes', 'y'].includes(raw.toLowerCase());
}

function localTimestamp() {
  return new Date().toISOString().replace(/[:.]/g, '-');
}

export const ENV = Object.freeze({
  baseUrl: trimTrailingSlash(__ENV.BASE_URL || 'http://localhost:8080'),
  upstreamBaseUrl: trimTrailingSlash(__ENV.UPSTREAM_BASE_URL || 'http://localhost:18080'),
  allowRemoteLoadTest: parseBoolean('ALLOW_REMOTE_LOAD_TEST', false),
  targetEnvironment: (__ENV.TARGET_ENVIRONMENT || 'local').trim().toLowerCase(),
  remoteTargetAllowlist: (__ENV.REMOTE_TARGET_ALLOWLIST || '')
    .split(',')
    .map((value) => value.trim().toLowerCase())
    .filter((value) => value.length > 0),
  vus: parsePositiveInt('LOAD_VUS', 1),
  duration: __ENV.TEST_DURATION || '10s',
  sleepSeconds: Number(__ENV.SLEEP_SECONDS || '0.1'),
  p95ThresholdMs: parseOptionalPositiveInt('P95_THRESHOLD_MS'),
  resultDir: __ENV.RESULT_DIR || 'performance/results',
  payloadBytes: parseNonNegativeInt('PAYLOAD_BYTES', 1024),
  mockDelayMs: parseNonNegativeInt('MOCK_DELAY_MS', 0),
  mockStatus: parsePositiveInt('MOCK_STATUS', 200),
  publicRoutePath: normalizePath(__ENV.PUBLIC_ROUTE_PATH || '/v2/blogs'),
  protectedRoutePath: normalizePath(__ENV.PROTECTED_ROUTE_PATH || '/v1/me'),
  forbiddenRoutePath: normalizePath(__ENV.FORBIDDEN_ROUTE_PATH || '/v1/admin/ping'),
  rateLimitPath: normalizePath(__ENV.RATE_LIMIT_PATH || '/v1/auth/login'),
  loginRateLimitPerMinute: parsePositiveInt('AUTH_LOGIN_RATE_LIMIT_PER_MINUTE', 10),
  runId: __ENV.RUN_ID || localTimestamp(),
  runIndex: __ENV.RUN_INDEX || '1',
  runRepeat: parsePositiveInt('RUN_REPEAT', 1),
  runOrder: __ENV.RUN_ORDER || 'direct-then-gateway',
  pairOrder: __ENV.PAIR_ORDER || __ENV.RUN_ORDER || 'direct-then-gateway',
  pairIndex: parsePositiveIntValue('PAIR_INDEX', __ENV.PAIR_INDEX, 1),
  measuredPosition: parseNonNegativeInt('MEASURED_POSITION', 1),
  warmupCompleted: parseBoolean('WARMUP_COMPLETED', false),
  commitSha: __ENV.COMMIT_SHA || 'unknown',
  gitDirty: parseBoolean('GIT_DIRTY', false),
  k6Version: __ENV.K6_VERSION || 'unknown',
});

if (!Number.isFinite(ENV.sleepSeconds) || ENV.sleepSeconds < 0) {
  throw new Error('SLEEP_SECONDS must be a non-negative number');
}

export function buildUrl(baseUrl, path, params = {}) {
  const query = Object.entries(params)
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
    .join('&');
  const url = `${trimTrailingSlash(baseUrl)}${normalizePath(path)}`;
  return query ? `${url}?${query}` : url;
}

export function commonMockParams(status = 200) {
  return {
    status,
    payloadBytes: ENV.payloadBytes,
    delayMs: ENV.mockDelayMs,
  };
}

export function commonHeaders(extra = {}) {
  return Object.assign(
    {
      Accept: 'application/json',
      'User-Agent': 'aandi-gateway-k6',
    },
    extra,
  );
}

function parseHttpUrl(url, name) {
  const match = /^(https?):\/\/(\[[^\]]+\]|[^/:?#]+)(?::\d+)?(?:[/?#]|$)/i.exec(url);
  if (!match) {
    throw new Error(`${name}=${url} is not a valid HTTP URL`);
  }

  const bracketedHost = match[2];
  return bracketedHost.startsWith('[') && bracketedHost.endsWith(']')
    ? bracketedHost.slice(1, -1).toLowerCase()
    : bracketedHost.toLowerCase();
}

export function assertLocalTargetAllowed(url, name) {
  const host = parseHttpUrl(url, name);
  const allowed = host === 'localhost'
    || host === '127.0.0.1'
    || host === '::1'
    || host === 'host.docker.internal';

  if (allowed) {
    return;
  }

  if (ENV.targetEnvironment === 'prod' || ENV.targetEnvironment === 'production') {
    throw new Error(`${name}=${url} is blocked because TARGET_ENVIRONMENT=${ENV.targetEnvironment}`);
  }

  const remoteAllowed = ENV.allowRemoteLoadTest
    && ENV.targetEnvironment === 'staging'
    && ENV.remoteTargetAllowlist.includes(host);
  if (remoteAllowed) {
    return;
  }

  throw new Error(
    `${name}=${url} is not an approved local or staging target. Set ALLOW_REMOTE_LOAD_TEST=true, TARGET_ENVIRONMENT=staging, and include the exact host in REMOTE_TARGET_ALLOWLIST.`,
  );
}

export function buildLoadOptions() {
  const thresholds = {
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
  };

  if (ENV.p95ThresholdMs !== null) {
    thresholds.http_req_duration = [`p(95)<${ENV.p95ThresholdMs}`];
  }

  return {
    vus: ENV.vus,
    duration: ENV.duration,
    thresholds,
    summaryTrendStats: ['min', 'avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  };
}

export function buildContractOptions() {
  return {
    scenarios: {
      contract: {
        executor: 'shared-iterations',
        vus: 1,
        iterations: 1,
        maxDuration: '1m',
      },
    },
    thresholds: {
      checks: ['rate>0.99'],
    },
    summaryTrendStats: ['min', 'avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  };
}

export function buildSkipOptions() {
  return {
    scenarios: {
      skipped: {
        executor: 'shared-iterations',
        vus: 1,
        iterations: 1,
        maxDuration: '10s',
      },
    },
    thresholds: {},
    summaryTrendStats: ['min', 'avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  };
}

export function buildRateLimitOptions(iterations, expectedAllowed, expectedRejected) {
  return {
    scenarios: {
      rate_limit: {
        executor: 'shared-iterations',
        vus: 1,
        iterations,
        maxDuration: '1m',
      },
    },
    thresholds: {
      checks: ['rate>0.99'],
      http_req_failed: ['rate<0.01'],
      rate_limit_allowed_responses: [`count==${expectedAllowed}`],
      rate_limit_rejected_responses: [`count==${expectedRejected}`],
    },
    summaryTrendStats: ['min', 'avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  };
}

export function baseConfig(target, routePath) {
  return {
    target,
    commitSha: ENV.commitSha,
    gitDirty: ENV.gitDirty,
    k6Version: ENV.k6Version,
    executor: 'constant-vus',
    vus: ENV.vus,
    duration: ENV.duration,
    sleepSeconds: ENV.sleepSeconds,
    payloadBytes: ENV.payloadBytes,
    mockDelayMs: ENV.mockDelayMs,
    mockStatus: ENV.mockStatus,
    routePath,
    runId: ENV.runId,
    runIndex: ENV.runIndex,
    runRepeat: ENV.runRepeat,
    runOrder: ENV.runOrder,
    pairOrder: ENV.pairOrder,
    pairIndex: ENV.pairIndex,
    measuredPosition: ENV.measuredPosition,
    warmupCompleted: ENV.warmupCompleted,
    p95ThresholdMs: ENV.p95ThresholdMs,
  };
}
