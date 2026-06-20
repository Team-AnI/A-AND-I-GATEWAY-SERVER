const fs = require('fs');
const http = require('http');
const path = require('path');

const ALLOWED_STATUSES = new Set([200, 400, 401, 403, 404, 429, 500]);
const MAX_PAYLOAD_BYTES = 1024 * 1024;
const MAX_DELAY_MS = 5000;
const PORT = parseStrictInteger(process.env.MOCK_PORT || '18080', 'MOCK_PORT', 1, 65535);

const successTemplate = readJsonTemplate('responses/success.json');
const errorTemplate = readJsonTemplate('responses/error.json');

function readJsonTemplate(relativePath) {
  const filePath = path.join(__dirname, relativePath);
  return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function parseStrictInteger(raw, name, min, max) {
  const value = String(raw);
  if (!/^\d+$/.test(value)) {
    throw new Error(`${name} must be an integer`);
  }
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed < min || parsed > max) {
    throw new Error(`${name} must be between ${min} and ${max}`);
  }
  return parsed;
}

function queryInteger(params, name, fallback, min, max) {
  const raw = params.get(name);
  if (raw === null || raw === '') {
    return fallback;
  }
  return parseStrictInteger(raw, name, min, max);
}

function errorForStatus(status) {
  const byStatus = {
    400: [13001, 'LOGIN_REQUEST_BODY_INVALID', 'mock bad request'],
    401: [11001, 'AUTHENTICATION_FAILED', 'mock unauthorized'],
    403: [12001, 'ACCESS_DENIED', 'mock forbidden'],
    404: [15001, 'ENDPOINT_NOT_ALLOWLISTED', 'mock not found'],
    429: [10003, 'LOGIN_RATE_LIMIT_EXCEEDED', 'mock rate limit'],
    500: [18801, 'INTERNAL_SERVER_ERROR', 'mock downstream error'],
  };
  const fields = byStatus[status] || byStatus[500];
  return {
    code: fields[0],
    value: fields[1],
    message: fields[2],
    alert: fields[2],
  };
}

function responseBody(req, parsed, status, payloadBytes, delayMs) {
  const timestamp = new Date().toISOString();
  if (status < 400) {
    const body = JSON.parse(JSON.stringify(successTemplate));
    body.data.method = req.method;
    body.data.path = parsed.pathname;
    body.data.payloadBytes = payloadBytes;
    body.data.delayMs = delayMs;
    body.data.payload = 'x'.repeat(payloadBytes);
    body.timestamp = timestamp;
    return body;
  }

  const body = JSON.parse(JSON.stringify(errorTemplate));
  body.error = errorForStatus(status);
  body.timestamp = timestamp;
  return body;
}

function writeJson(res, status, body) {
  const raw = JSON.stringify(body);
  res.writeHead(status, {
    'Content-Type': 'application/json; charset=utf-8',
    'Content-Length': Buffer.byteLength(raw),
    'X-Mock-Upstream': 'true',
  });
  res.end(raw);
}

function badRequest(res, message) {
  writeJson(res, 400, {
    success: false,
    data: null,
    error: {
      code: 13001,
      message,
      value: 'MOCK_REQUEST_INVALID',
      alert: message,
    },
    timestamp: new Date().toISOString(),
  });
}

const server = http.createServer((req, res) => {
  const parsed = new URL(req.url, 'http://mock-upstream');
  if (parsed.pathname === '/health') {
    writeJson(res, 200, { status: 'UP' });
    return;
  }

  let status;
  let payloadBytes;
  let delayMs;
  try {
    status = queryInteger(parsed.searchParams, 'status', 200, 100, 599);
    payloadBytes = queryInteger(parsed.searchParams, 'payloadBytes', 0, 0, MAX_PAYLOAD_BYTES);
    delayMs = queryInteger(parsed.searchParams, 'delayMs', 0, 0, MAX_DELAY_MS);
  } catch (error) {
    badRequest(res, error.message);
    return;
  }

  if (!ALLOWED_STATUSES.has(status)) {
    badRequest(res, 'status must be one of 200, 400, 401, 403, 404, 429, 500');
    return;
  }

  const body = responseBody(req, parsed, status, payloadBytes, delayMs);
  setTimeout(() => writeJson(res, status, body), delayMs);
});

server.listen(PORT, '0.0.0.0', () => {
  console.log(`mock-upstream listening on ${PORT}`);
});
