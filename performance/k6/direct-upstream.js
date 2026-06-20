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

const TEST_NAME = 'direct-upstream';

export const options = buildLoadOptions();

export function setup() {
  assertLocalTargetAllowed(ENV.upstreamBaseUrl, 'UPSTREAM_BASE_URL');
  return {
    url: buildUrl(ENV.upstreamBaseUrl, ENV.publicRoutePath, commonMockParams(200)),
  };
}

export default function (data) {
  const res = http.get(data.url, {
    headers: commonHeaders(),
    tags: {
      target: 'direct',
      route: ENV.publicRoutePath,
    },
  });

  successEnvelopeChecks(res, 200);
  sleep(0.1);
}

export const handleSummary = makeHandleSummary(TEST_NAME, () => ({
  config: Object.assign(baseConfig('direct', ENV.publicRoutePath), {
    baseUrl: ENV.upstreamBaseUrl,
    mockStatus: 200,
  }),
}));
