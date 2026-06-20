import http from 'k6/http';
import { sleep } from 'k6';
import {
  ENV,
  assertLocalTargetAllowed,
  baseConfig,
  buildLoadOptions,
  buildSkipOptions,
  buildUrl,
  commonHeaders,
  commonMockParams,
} from './config.js';
import { authHeaders, userTokenFromEnv } from './lib/auth.js';
import { successEnvelopeChecks } from './lib/checks.js';
import { makeHandleSummary } from './lib/summary.js';

const TEST_NAME = 'gateway-protected-route';
const TOKEN = userTokenFromEnv();

export const options = TOKEN ? buildLoadOptions() : buildSkipOptions();

export function setup() {
  if (!TOKEN) {
    if (__ENV.SKIP_AUTH_SCENARIOS !== 'true') {
      throw new Error('USER_ACCESS_TOKEN is required unless SKIP_AUTH_SCENARIOS=true');
    }
    return {
      skipped: true,
      reason: 'SKIPPED: token not supplied',
    };
  }

  assertLocalTargetAllowed(ENV.baseUrl, 'BASE_URL');
  return {
    url: buildUrl(ENV.baseUrl, ENV.protectedRoutePath, commonMockParams(200)),
  };
}

export default function (data) {
  if (data.skipped) {
    console.warn(data.reason);
    return;
  }

  const res = http.get(data.url, {
    headers: commonHeaders(authHeaders(TOKEN)),
    tags: {
      target: 'gateway',
      route: ENV.protectedRoutePath,
    },
  });

  successEnvelopeChecks(res, 200);
  sleep(ENV.sleepSeconds);
}

export const handleSummary = makeHandleSummary(TEST_NAME, () => ({
  note: TOKEN ? '' : 'SKIPPED: token not supplied',
  config: Object.assign(baseConfig('gateway', ENV.protectedRoutePath), {
    baseUrl: ENV.baseUrl,
    upstreamBaseUrl: ENV.upstreamBaseUrl,
    mockStatus: TOKEN ? 200 : null,
    tokenSupplied: Boolean(TOKEN),
  }),
}));
