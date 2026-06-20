import { check } from 'k6';

function headerValue(headers, name) {
  const lowerName = name.toLowerCase();
  const key = Object.keys(headers || {}).find((candidate) => candidate.toLowerCase() === lowerName);
  return key ? headers[key] : '';
}

export function parseJson(res) {
  try {
    return res.json();
  } catch (error) {
    return null;
  }
}

export function hasJsonContentType(res) {
  return headerValue(res.headers, 'content-type').toLowerCase().includes('application/json');
}

export function commonEnvelopeChecks(res, expected) {
  const body = parseJson(res);
  return check(res, {
    [`status is ${expected.status}`]: (r) => r.status === expected.status,
    'content-type is application/json': hasJsonContentType,
    'body is json': () => body !== null,
    'success flag is present': () => typeof body?.success === 'boolean',
    'data field is present': () => body !== null && Object.prototype.hasOwnProperty.call(body, 'data'),
    'timestamp field is present': () => typeof body?.timestamp === 'string' && body.timestamp.length > 0,
    'error shape matches status': () => {
      if (expected.status < 400) {
        return body?.error === null || body?.error === undefined;
      }
      return typeof body?.error?.code === 'number'
        && typeof body?.error?.message === 'string'
        && typeof body?.error?.value === 'string'
        && typeof body?.error?.alert === 'string';
    },
  });
}

export function successEnvelopeChecks(res, expectedStatus = 200) {
  const body = parseJson(res);
  return check(res, {
    [`status is ${expectedStatus}`]: (r) => r.status === expectedStatus,
    'content-type is application/json': hasJsonContentType,
    'success is true': () => body?.success === true,
    'data is present': () => body?.data !== null && body?.data !== undefined,
    'error is empty': () => body?.error === null || body?.error === undefined,
    'timestamp field is present': () => typeof body?.timestamp === 'string' && body.timestamp.length > 0,
  });
}

export function errorEnvelopeChecks(res, expectedStatus, expectedValue) {
  const body = parseJson(res);
  return check(res, {
    [`status is ${expectedStatus}`]: (r) => r.status === expectedStatus,
    'content-type is application/json': hasJsonContentType,
    'success is false': () => body?.success === false,
    'data is null': () => body?.data === null,
    [`error value is ${expectedValue}`]: () => body?.error?.value === expectedValue,
    'error code is not internal server error': () => body?.error?.code !== 18801,
    'timestamp field is present': () => typeof body?.timestamp === 'string' && body.timestamp.length > 0,
  });
}

export function responseDoesNotExposeSecret(res, secret) {
  if (!secret) {
    return true;
  }
  const body = typeof res.body === 'string' ? res.body : '';
  const raw = secret.replace(/^bearer\s+/i, '');
  return !body.includes(secret)
    && !body.includes(raw)
    && !headerValue(res.headers, 'authorization')
    && !headerValue(res.headers, 'authenticate');
}
