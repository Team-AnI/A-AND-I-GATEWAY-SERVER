import http from 'k6/http';
import { sleep } from 'k6';
import {
  ENV,
  assertLocalTargetAllowed,
  baseConfig,
  buildLoadOptions,
  buildUrl,
  commonHeaders,
  commonMockParams,
} from './config.js';
import { successEnvelopeChecks } from './lib/checks.js';
import { makeHandleSummary } from './lib/summary.js';

const TEST_NAME = 'gateway-public-route';

export const options = buildLoadOptions();

export function setup() {
  assertLocalTargetAllowed(ENV.baseUrl, 'BASE_URL');
  return {
    url: buildUrl(ENV.baseUrl, ENV.publicRoutePath, commonMockParams(200)),
  };
}

export default function (data) {
  const res = http.get(data.url, {
    headers: commonHeaders(),
    tags: {
      target: 'gateway',
      route: ENV.publicRoutePath,
    },
  });

  successEnvelopeChecks(res, 200);
  sleep(0.1);
}

export const handleSummary = makeHandleSummary(TEST_NAME, () => ({
  config: Object.assign(baseConfig('gateway', ENV.publicRoutePath), {
    baseUrl: ENV.baseUrl,
    upstreamBaseUrl: ENV.upstreamBaseUrl,
    mockStatus: 200,
  }),
}));
