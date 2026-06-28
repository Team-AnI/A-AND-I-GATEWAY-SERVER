import http from 'k6/http';
import { sleep } from 'k6';
import {
  ENV,
  assertLocalRegressionTargetAllowed,
  baseConfig,
  buildLoadOptions,
  buildUrl,
  commonHeaders,
  commonMockParams,
} from './config.js';
import { mockSuccessChecks } from './lib/checks.js';
import { makeHandleSummary } from './lib/summary.js';

const TARGET = (__ENV.OVERHEAD_TARGET || 'gateway').trim().toLowerCase();
const LOGGING_MODE = (__ENV.OVERHEAD_LOGGING_MODE || 'enabled').trim().toLowerCase();
if (!['direct', 'gateway'].includes(TARGET)) {
  throw new Error('OVERHEAD_TARGET must be direct or gateway');
}
if (!['enabled', 'disabled'].includes(LOGGING_MODE)) {
  throw new Error('OVERHEAD_LOGGING_MODE must be enabled or disabled');
}

const TEST_NAME = `gateway-logging-overhead-${LOGGING_MODE}-${TARGET}`;
const BASE_URL = TARGET === 'direct' ? ENV.upstreamBaseUrl : ENV.baseUrl;

export const options = buildLoadOptions();

export function setup() {
  assertLocalRegressionTargetAllowed(ENV.baseUrl, 'BASE_URL');
  assertLocalRegressionTargetAllowed(ENV.upstreamBaseUrl, 'UPSTREAM_BASE_URL', { allowMockUpstream: true });
  assertLocalRegressionTargetAllowed(ENV.downstreamUrl, 'DOWNSTREAM_URL', { allowMockUpstream: true });
  return {
    url: buildUrl(BASE_URL, ENV.publicRoutePath, commonMockParams(200)),
  };
}

export default function (data) {
  const res = http.get(data.url, {
    headers: commonHeaders(),
    tags: {
      scenario: 'logging-overhead',
      target: TARGET,
      loggingMode: LOGGING_MODE,
      route: ENV.publicRoutePath,
    },
  });

  mockSuccessChecks(res, {
    path: ENV.publicRoutePath,
    payloadBytes: ENV.payloadBytes,
    delayMs: ENV.mockDelayMs,
  });
  sleep(ENV.sleepSeconds);
}

export const handleSummary = makeHandleSummary(TEST_NAME, () => ({
  config: Object.assign(baseConfig(TARGET, ENV.publicRoutePath), {
    scenario: 'logging-overhead',
    loggingMode: LOGGING_MODE,
    overheadTarget: TARGET,
    baseUrl: BASE_URL,
    gatewayBaseUrl: ENV.baseUrl,
    upstreamBaseUrl: ENV.upstreamBaseUrl,
    downstreamUrl: ENV.downstreamUrl,
    mockStatus: 200,
  }),
}));
