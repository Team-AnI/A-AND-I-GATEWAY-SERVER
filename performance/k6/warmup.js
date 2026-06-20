import http from 'k6/http';
import { check, sleep } from 'k6';
import {
  ENV,
  assertLocalTargetAllowed,
  buildUrl,
  commonHeaders,
  commonMockParams,
} from './config.js';

export const options = {
  scenarios: {
    warmup: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 4,
      maxDuration: '30s',
    },
  },
  thresholds: {
    checks: ['rate>0.99'],
  },
};

export function setup() {
  assertLocalTargetAllowed(ENV.upstreamBaseUrl, 'UPSTREAM_BASE_URL');
  assertLocalTargetAllowed(ENV.baseUrl, 'BASE_URL');
  return {
    directUrl: buildUrl(ENV.upstreamBaseUrl, ENV.publicRoutePath, commonMockParams(200)),
    gatewayUrl: buildUrl(ENV.baseUrl, ENV.publicRoutePath, commonMockParams(200)),
  };
}

export default function (data) {
  const direct = http.get(data.directUrl, { headers: commonHeaders(), tags: { phase: 'warmup', target: 'direct' } });
  const gateway = http.get(data.gatewayUrl, { headers: commonHeaders(), tags: { phase: 'warmup', target: 'gateway' } });
  check(direct, { 'warmup_direct_200': (res) => res.status === 200 });
  check(gateway, { 'warmup_gateway_200': (res) => res.status === 200 });
  sleep(ENV.sleepSeconds);
}
