import http from 'k6/http';
import {
  ENV,
  assertLocalTargetAllowed,
  baseConfig,
  buildContractOptions,
  buildUrl,
  commonHeaders,
  commonMockParams,
} from './config.js';
import { adminTokenFromEnv, authHeaders, tokenRole, userTokenFromEnv } from './lib/auth.js';
import {
  commonEnvelopeChecks,
  errorEnvelopeChecks,
  responseDoesNotExposeSecret,
} from './lib/checks.js';
import { makeHandleSummary } from './lib/summary.js';

const TEST_NAME = 'gateway-error-contract';
const USER_TOKEN = userTokenFromEnv();
const ADMIN_TOKEN = adminTokenFromEnv();

http.setResponseCallback(http.expectedStatuses({ min: 200, max: 599 }));

export const options = buildContractOptions();

export function setup() {
  assertLocalTargetAllowed(ENV.baseUrl, 'BASE_URL');
  if ((!USER_TOKEN || !ADMIN_TOKEN) && __ENV.SKIP_AUTH_SCENARIOS !== 'true') {
    throw new Error('USER_ACCESS_TOKEN and ADMIN_ACCESS_TOKEN are required unless SKIP_AUTH_SCENARIOS=true');
  }
  return {
    userTokenSupplied: Boolean(USER_TOKEN),
    adminTokenSupplied: Boolean(ADMIN_TOKEN),
    userTokenRole: tokenRole(USER_TOKEN),
    adminTokenRole: tokenRole(ADMIN_TOKEN),
  };
}

function checkUnauthorized() {
  const res = http.get(buildUrl(ENV.baseUrl, ENV.protectedRoutePath), {
    headers: commonHeaders(),
    tags: {
      contract: '401',
      route: ENV.protectedRoutePath,
    },
  });

  errorEnvelopeChecks(res, 401, 'AUTHENTICATION_FAILED');
  commonEnvelopeChecks(res, { status: 401 });
  if (!responseDoesNotExposeSecret(res, USER_TOKEN)) {
    throw new Error('401 response exposed an authorization value');
  }
}

function checkUserProtected(data) {
  if (!data.userTokenSupplied) {
    console.warn('SKIPPED: USER token not supplied for protected route check');
    return;
  }

  const res = http.get(buildUrl(ENV.baseUrl, ENV.protectedRoutePath, commonMockParams(200)), {
    headers: commonHeaders(authHeaders(USER_TOKEN)),
    tags: {
      contract: 'user-protected-200',
      route: ENV.protectedRoutePath,
    },
  });

  commonEnvelopeChecks(res, { status: 200 });
}

function checkForbidden(data) {
  if (!data.userTokenSupplied) {
    console.warn('SKIPPED: USER token not supplied for 403 contract check');
    return;
  }

  const res = http.get(buildUrl(ENV.baseUrl, ENV.forbiddenRoutePath), {
    headers: commonHeaders(authHeaders(USER_TOKEN)),
    tags: {
      contract: '403',
      route: ENV.forbiddenRoutePath,
    },
  });

  errorEnvelopeChecks(res, 403, 'ACCESS_DENIED');
  commonEnvelopeChecks(res, { status: 403 });
  if (!responseDoesNotExposeSecret(res, USER_TOKEN)) {
    throw new Error('403 response exposed an authorization value');
  }
}

function checkAdminAllowed(data) {
  if (!data.adminTokenSupplied) {
    console.warn('SKIPPED: ADMIN token not supplied for admin route check');
    return;
  }

  const res = http.get(buildUrl(ENV.baseUrl, ENV.forbiddenRoutePath, commonMockParams(200)), {
    headers: commonHeaders(authHeaders(ADMIN_TOKEN)),
    tags: {
      contract: 'admin-200',
      route: ENV.forbiddenRoutePath,
    },
  });

  commonEnvelopeChecks(res, { status: 200 });
}

function checkNotFound() {
  const res = http.get(buildUrl(ENV.baseUrl, `/__k6/not-allowlisted/${ENV.runId}/${ENV.runIndex}`), {
    headers: commonHeaders(),
    tags: {
      contract: '404',
      route: 'not-allowlisted',
    },
  });

  errorEnvelopeChecks(res, 404, 'ENDPOINT_NOT_ALLOWLISTED');
  commonEnvelopeChecks(res, { status: 404 });
}

function downstream_500_passthrough() {
  const res = http.get(buildUrl(ENV.baseUrl, ENV.publicRoutePath, commonMockParams(500)), {
    headers: commonHeaders(),
    tags: {
      contract: 'downstream_500_passthrough',
      route: ENV.publicRoutePath,
    },
  });

  commonEnvelopeChecks(res, { status: 500 });
}

function checkGatewayConnectionFailure(data) {
  if (!data.userTokenSupplied) {
    console.warn('SKIPPED: USER token not supplied for 502 contract check');
    return;
  }

  const res = http.get(buildUrl(ENV.baseUrl, '/v2/report', commonMockParams(200)), {
    headers: commonHeaders(authHeaders(USER_TOKEN)),
    tags: {
      contract: '502',
      route: '/v2/report',
    },
  });

  errorEnvelopeChecks(res, 502, 'DOWNSTREAM_SERVICE_UNAVAILABLE');
}

export default function (data) {
  checkUnauthorized();
  checkUserProtected(data);
  checkForbidden(data);
  checkAdminAllowed(data);
  checkNotFound();
  downstream_500_passthrough();
  if (__ENV.EXPECT_REPORT_502 === 'true') {
    checkGatewayConnectionFailure(data);
  }
}

export const handleSummary = makeHandleSummary(TEST_NAME, () => ({
  note: USER_TOKEN && ADMIN_TOKEN ? '' : 'auth checks require USER_ACCESS_TOKEN and ADMIN_ACCESS_TOKEN',
  config: Object.assign(baseConfig('gateway', 'error-contract'), {
    baseUrl: ENV.baseUrl,
    protectedRoutePath: ENV.protectedRoutePath,
    forbiddenRoutePath: ENV.forbiddenRoutePath,
    publicRoutePath: ENV.publicRoutePath,
    userTokenSupplied: Boolean(USER_TOKEN),
    adminTokenSupplied: Boolean(ADMIN_TOKEN),
    suppliedUserTokenRole: tokenRole(USER_TOKEN) || 'unknown',
    suppliedAdminTokenRole: tokenRole(ADMIN_TOKEN) || 'unknown',
    expectReport502: __ENV.EXPECT_REPORT_502 === 'true',
  }),
}));
