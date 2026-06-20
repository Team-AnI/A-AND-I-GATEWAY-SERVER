import exec from 'k6/execution';
import http from 'k6/http';
import { Counter } from 'k6/metrics';
import {
  ENV,
  assertLocalTargetAllowed,
  baseConfig,
  buildRateLimitOptions,
  buildUrl,
  commonHeaders,
} from './config.js';
import { errorEnvelopeChecks, successEnvelopeChecks } from './lib/checks.js';
import { makeHandleSummary } from './lib/summary.js';

const TEST_NAME = 'gateway-rate-limit';
const ITERATIONS = ENV.loginRateLimitPerMinute + 2;
const allowedResponses = new Counter('rate_limit_allowed_responses');
const rejectedResponses = new Counter('rate_limit_rejected_responses');

http.setResponseCallback(http.expectedStatuses({ min: 200, max: 399 }, 429));

export const options = buildRateLimitOptions(ITERATIONS);

export function setup() {
  assertLocalTargetAllowed(ENV.baseUrl, 'BASE_URL');
  return {
    username: `k6-rate-limit-${ENV.runId}-${ENV.runIndex}`,
    url: buildUrl(ENV.baseUrl, ENV.rateLimitPath),
  };
}

export default function (data) {
  const iteration = exec.scenario.iterationInTest;
  const body = JSON.stringify({
    username: data.username,
    password: 'local-performance-password',
  });

  const res = http.post(data.url, body, {
    headers: commonHeaders({
      'Content-Type': 'application/json',
    }),
    tags: {
      target: 'gateway',
      route: ENV.rateLimitPath,
    },
  });

  if (res.status >= 200 && res.status < 300) {
    allowedResponses.add(1);
  }
  if (res.status === 429) {
    rejectedResponses.add(1);
  }

  if (iteration < ENV.loginRateLimitPerMinute) {
    successEnvelopeChecks(res, 200);
    return;
  }

  errorEnvelopeChecks(res, 429, 'LOGIN_RATE_LIMIT_EXCEEDED');
}

export const handleSummary = makeHandleSummary(TEST_NAME, () => ({
  note: 'Current gateway rate limit uses in-memory keys, not Redis keys.',
  config: Object.assign(baseConfig('gateway', ENV.rateLimitPath), {
    baseUrl: ENV.baseUrl,
    expectedLoginRateLimitPerMinute: ENV.loginRateLimitPerMinute,
    expectedIterations: ITERATIONS,
    rateLimitKeyPattern: 'login:<remote-ip>:<username>',
  }),
}));
