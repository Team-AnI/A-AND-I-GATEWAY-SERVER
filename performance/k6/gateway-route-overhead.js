import http from 'k6/http';
import { sleep } from 'k6';
import {
  ENV,
  assertLocalRegressionTargetAllowed,
  baseConfig,
  buildLoadOptions,
  buildSkipOptions,
  buildUrl,
  commonHeaders,
  commonMockParams,
} from './config.js';
import { authHeaders, userTokenFromEnv } from './lib/auth.js';
import { mockSuccessChecks, successEnvelopeChecks } from './lib/checks.js';
import { makeHandleSummary } from './lib/summary.js';

const TARGET = (__ENV.OVERHEAD_TARGET || 'gateway').trim().toLowerCase();
const ROUTE_KIND = (__ENV.OVERHEAD_ROUTE_KIND || 'public').trim().toLowerCase();
if (!['direct', 'gateway'].includes(TARGET)) {
  throw new Error('OVERHEAD_TARGET must be direct or gateway');
}
if (!['public', 'protected'].includes(ROUTE_KIND)) {
  throw new Error('OVERHEAD_ROUTE_KIND must be public or protected');
}

const TOKEN = userTokenFromEnv();
const TOKEN_REQUIRED = TARGET === 'gateway' && ROUTE_KIND === 'protected';
const ROUTE_PATH = ROUTE_KIND === 'protected' ? ENV.protectedRoutePath : ENV.publicRoutePath;
const BASE_URL = TARGET === 'direct' ? ENV.upstreamBaseUrl : ENV.baseUrl;
const TEST_NAME = `gateway-route-overhead-${ROUTE_KIND}-${TARGET}`;

export const options = TOKEN_REQUIRED && !TOKEN ? buildSkipOptions() : buildLoadOptions();

export function setup() {
  assertLocalRegressionTargetAllowed(ENV.baseUrl, 'BASE_URL');
  assertLocalRegressionTargetAllowed(ENV.upstreamBaseUrl, 'UPSTREAM_BASE_URL', { allowMockUpstream: true });
  assertLocalRegressionTargetAllowed(ENV.downstreamUrl, 'DOWNSTREAM_URL', { allowMockUpstream: true });

  if (TOKEN_REQUIRED && !TOKEN) {
    if (__ENV.SKIP_AUTH_SCENARIOS !== 'true') {
      throw new Error('USER_ACCESS_TOKEN is required for protected Gateway overhead unless SKIP_AUTH_SCENARIOS=true');
    }
    return {
      skipped: true,
      reason: 'SKIPPED: token not supplied',
    };
  }

  return {
    url: buildUrl(BASE_URL, ROUTE_PATH, commonMockParams(200)),
  };
}

export default function (data) {
  if (data.skipped) {
    console.warn(data.reason);
    return;
  }

  const headers = TOKEN_REQUIRED ? commonHeaders(authHeaders(TOKEN)) : commonHeaders();
  const res = http.get(data.url, {
    headers,
    tags: {
      scenario: 'route-overhead',
      target: TARGET,
      routeKind: ROUTE_KIND,
      route: ROUTE_PATH,
    },
  });

  if (TARGET === 'gateway' && ROUTE_KIND === 'protected') {
    successEnvelopeChecks(res, 200);
  } else {
    mockSuccessChecks(res, {
      path: ROUTE_PATH,
      payloadBytes: ENV.payloadBytes,
      delayMs: ENV.mockDelayMs,
    });
  }
  sleep(ENV.sleepSeconds);
}

export const handleSummary = makeHandleSummary(TEST_NAME, () => ({
  note: TOKEN_REQUIRED && !TOKEN ? 'SKIPPED: token not supplied' : '',
  config: Object.assign(baseConfig(TARGET, ROUTE_PATH), {
    scenario: 'route-overhead',
    routeKind: ROUTE_KIND,
    overheadTarget: TARGET,
    baseUrl: BASE_URL,
    gatewayBaseUrl: ENV.baseUrl,
    upstreamBaseUrl: ENV.upstreamBaseUrl,
    downstreamUrl: ENV.downstreamUrl,
    mockStatus: TOKEN_REQUIRED && !TOKEN ? null : 200,
    tokenSupplied: Boolean(TOKEN),
  }),
}));
