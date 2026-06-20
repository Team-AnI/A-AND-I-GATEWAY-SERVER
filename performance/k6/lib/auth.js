import encoding from 'k6/encoding';

export function tokenFromEnv() {
  return (__ENV.ACCESS_TOKEN || __ENV.TEST_JWT || '').trim();
}

export function authHeaderValue(token = tokenFromEnv()) {
  if (!token) {
    return '';
  }
  return token.toLowerCase().startsWith('bearer ') ? token : `Bearer ${token}`;
}

export function authHeaders(token = tokenFromEnv()) {
  const value = authHeaderValue(token);
  return value ? { Authorization: value } : {};
}

export function decodedJwtPayload(token = tokenFromEnv()) {
  const raw = token.replace(/^bearer\s+/i, '');
  const parts = raw.split('.');
  if (parts.length < 2) {
    return {};
  }

  try {
    return JSON.parse(encoding.b64decode(parts[1], 'rawurl', 's'));
  } catch (error) {
    return {};
  }
}

export function tokenRole(token = tokenFromEnv()) {
  const payload = decodedJwtPayload(token);
  return typeof payload.role === 'string' ? payload.role : '';
}
