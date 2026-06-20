export function trimTrailingSlash(value) {
  return value.replace(/\/+$/, '');
}

export function normalizePath(value) {
  if (!value || value === '/') {
    return '/';
  }
  return value.startsWith('/') ? value : `/${value}`;
}

function parsePositiveInt(name, fallback) {
  const raw = __ENV[name];
  if (raw === undefined || raw === '') {
    return fallback;
  }
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new Error(`${name} must be a positive integer`);
  }
  return parsed;
}

function parseNonNegativeInt(name, fallback) {
  const raw = __ENV[name];
  if (raw === undefined || raw === '') {
    return fallback;
  }
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed < 0) {
    throw new Error(`${name} must be a non-negative integer`);
  }
  return parsed;
}

function parseOptionalPositiveInt(name) {
  const raw = __ENV[name];
  if (raw === undefined || raw === '') {
    return null;
  }
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
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
  vus: parsePositiveInt('LOAD_VUS', 1),
  duration: __ENV.TEST_DURATION || '10s',
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
  commitSha: __ENV.COMMIT_SHA || 'unknown',
});

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

export function assertLocalTargetAllowed(url, name) {
  if (ENV.allowRemoteLoadTest) {
    return;
  }

  const match = String(url).match(/^[a-z][a-z0-9+.-]*:\/\/(\[[^\]]+]|[^/:?#]+)/i);
  const host = match ? match[1].replace(/^\[/, '').replace(/\]$/, '').toLowerCase() : '';
  const allowed = host === 'localhost'
    || host === '127.0.0.1'
    || host === '::1'
    || host === 'host.docker.internal';

  if (!allowed) {
    throw new Error(
      `${name}=${url} is not a local target. Set ALLOW_REMOTE_LOAD_TEST=true only for an approved non-production target.`,
    );
  }
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

export function buildRateLimitOptions(iterations) {
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
    },
    summaryTrendStats: ['min', 'avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  };
}

export function baseConfig(target, routePath) {
  return {
    target,
    commitSha: ENV.commitSha,
    vus: ENV.vus,
    duration: ENV.duration,
    payloadBytes: ENV.payloadBytes,
    mockDelayMs: ENV.mockDelayMs,
    mockStatus: ENV.mockStatus,
    routePath,
    runId: ENV.runId,
    runIndex: ENV.runIndex,
    runRepeat: ENV.runRepeat,
    runOrder: ENV.runOrder,
    p95ThresholdMs: ENV.p95ThresholdMs,
  };
}
