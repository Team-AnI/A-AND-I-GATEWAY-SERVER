import http from 'k6/http';
import { check, sleep } from 'k6';
import {
  ENV,
  assertLocalTargetAllowed,
  buildUrl,
  commonHeaders,
  commonMockParams,
} from './config.js';
import { authHeaders, userTokenFromEnv } from './lib/auth.js';

const USER_TOKEN = userTokenFromEnv();

export const options = {
  scenarios: {
    warmup: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 10,
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
  if (!USER_TOKEN && __ENV.SKIP_AUTH_SCENARIOS !== 'true') {
    throw new Error('USER_ACCESS_TOKEN is required for warm-up unless SKIP_AUTH_SCENARIOS=true');
  }
  return {
    directUrl: buildUrl(ENV.upstreamBaseUrl, ENV.publicRoutePath, commonMockParams(200)),
    gatewayUrl: buildUrl(ENV.baseUrl, ENV.publicRoutePath, commonMockParams(200)),
    protectedUrl: buildUrl(ENV.baseUrl, ENV.protectedRoutePath, commonMockParams(200)),
  };
}

export default function (data) {
  const direct = http.get(data.directUrl, { headers: commonHeaders(), tags: { phase: 'warmup', target: 'direct' } });
  const gateway = http.get(data.gatewayUrl, { headers: commonHeaders(), tags: { phase: 'warmup', target: 'gateway' } });
  const protectedRoute = USER_TOKEN
    ? http.get(data.protectedUrl, {
      headers: commonHeaders(authHeaders(USER_TOKEN)),
      tags: { phase: 'warmup', target: 'gateway-protected' },
    })
    : null;
  check(direct, { 'warmup_direct_200': (res) => res.status === 200 });
  check(gateway, { 'warmup_gateway_200': (res) => res.status === 200 });
  if (protectedRoute) {
    check(protectedRoute, { 'warmup_user_me_200': (res) => res.status === 200 });
  }
  sleep(ENV.sleepSeconds);
}
