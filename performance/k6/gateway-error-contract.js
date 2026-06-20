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
import { authHeaders, tokenFromEnv, tokenRole } from './lib/auth.js';
import {
  commonEnvelopeChecks,
  errorEnvelopeChecks,
  responseDoesNotExposeSecret,
} from './lib/checks.js';
import { makeHandleSummary } from './lib/summary.js';

const TEST_NAME = 'gateway-error-contract';
const TOKEN = tokenFromEnv();

http.setResponseCallback(http.expectedStatuses({ min: 200, max: 599 }));

export const options = buildContractOptions();

export function setup() {
  assertLocalTargetAllowed(ENV.baseUrl, 'BASE_URL');
  return {
    tokenSupplied: Boolean(TOKEN),
    tokenRole: tokenRole(TOKEN),
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
  if (!responseDoesNotExposeSecret(res, TOKEN)) {
    throw new Error('401 response exposed an authorization value');
  }
}

function checkForbidden(data) {
  if (!data.tokenSupplied) {
    console.warn('SKIPPED: token not supplied for 403 contract check');
    return;
  }
  if (data.tokenRole === 'ADMIN') {
    console.warn('SKIPPED: supplied token has ADMIN role, cannot verify insufficient-permission 403');
    return;
  }

  const res = http.get(buildUrl(ENV.baseUrl, ENV.forbiddenRoutePath), {
    headers: commonHeaders(authHeaders(TOKEN)),
    tags: {
      contract: '403',
      route: ENV.forbiddenRoutePath,
    },
  });

  errorEnvelopeChecks(res, 403, 'ACCESS_DENIED');
  commonEnvelopeChecks(res, { status: 403 });
  if (!responseDoesNotExposeSecret(res, TOKEN)) {
    throw new Error('403 response exposed an authorization value');
  }
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

function checkDownstream5xx() {
  const res = http.get(buildUrl(ENV.baseUrl, ENV.publicRoutePath, commonMockParams(500)), {
    headers: commonHeaders(),
    tags: {
      contract: '5xx',
      route: ENV.publicRoutePath,
    },
  });

  commonEnvelopeChecks(res, { status: 500 });
}

export default function (data) {
  checkUnauthorized();
  checkForbidden(data);
  checkNotFound();
  checkDownstream5xx();
}

export const handleSummary = makeHandleSummary(TEST_NAME, () => ({
  note: TOKEN ? '' : '403 check skipped because token was not supplied',
  config: Object.assign(baseConfig('gateway', 'error-contract'), {
    baseUrl: ENV.baseUrl,
    protectedRoutePath: ENV.protectedRoutePath,
    forbiddenRoutePath: ENV.forbiddenRoutePath,
    publicRoutePath: ENV.publicRoutePath,
    tokenSupplied: Boolean(TOKEN),
    suppliedTokenRole: tokenRole(TOKEN) || 'unknown',
  }),
}));
