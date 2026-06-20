import { ENV } from './environment.js';

function safeName(value) {
  return String(value).replace(/[^a-zA-Z0-9_.-]/g, '-');
}

function metricValues(data, metricName) {
  return data.metrics?.[metricName]?.values || {};
}

function numberOrNull(value) {
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

function compactMetric(data, metricName) {
  const values = metricValues(data, metricName);
  return {
    count: numberOrNull(values.count),
    rate: numberOrNull(values.rate),
    avg: numberOrNull(values.avg),
    min: numberOrNull(values.min),
    med: numberOrNull(values.med),
    max: numberOrNull(values.max),
    p90: numberOrNull(values['p(90)']),
    p95: numberOrNull(values['p(95)']),
    p99: numberOrNull(values['p(99)']),
  };
}

function markdownValue(value, suffix = '') {
  if (value === null || value === undefined || Number.isNaN(value)) {
    return 'n/a';
  }
  if (typeof value === 'number') {
    return `${value.toFixed(3)}${suffix}`;
  }
  return `${value}${suffix}`;
}

function toMarkdown(summary) {
  const duration = summary.metrics.http_req_duration;
  const requests = summary.metrics.http_reqs;
  const failed = summary.metrics.http_req_failed;
  const checks = summary.metrics.checks;

  return [
    `# ${summary.testName}`,
    '',
    `- Generated At: ${summary.generatedAt}`,
    `- Commit SHA: ${summary.config.commitSha}`,
    `- Target: ${summary.config.target}`,
    `- Route Path: ${summary.config.routePath}`,
    `- VUs: ${summary.config.vus}`,
    `- Duration: ${summary.config.duration}`,
    `- Payload Bytes: ${summary.config.payloadBytes}`,
    `- Mock Delay Ms: ${summary.config.mockDelayMs}`,
    `- Run: ${summary.config.runId} / ${summary.config.runIndex}`,
    '',
    '| Metric | Value |',
    '| --- | ---: |',
    `| HTTP duration P50 | ${markdownValue(duration.med, ' ms')} |`,
    `| HTTP duration P95 | ${markdownValue(duration.p95, ' ms')} |`,
    `| HTTP duration P99 | ${markdownValue(duration.p99, ' ms')} |`,
    `| Requests/sec | ${markdownValue(requests.rate)} |`,
    `| HTTP failed rate | ${markdownValue(failed.rate)} |`,
    `| Check rate | ${markdownValue(checks.rate)} |`,
    '',
    summary.note ? `> ${summary.note}` : '',
    '',
  ].filter((line, index, lines) => line !== '' || lines[index - 1] !== '').join('\n');
}

function buildSummary(data, testName, metadata) {
  const config = Object.assign({}, metadata.config || {});
  return {
    schemaVersion: 1,
    testName,
    generatedAt: new Date().toISOString(),
    note: metadata.note || '',
    config,
    metrics: {
      http_req_duration: compactMetric(data, 'http_req_duration'),
      http_reqs: compactMetric(data, 'http_reqs'),
      http_req_failed: compactMetric(data, 'http_req_failed'),
      checks: compactMetric(data, 'checks'),
      rate_limit_allowed_responses: compactMetric(data, 'rate_limit_allowed_responses'),
      rate_limit_rejected_responses: compactMetric(data, 'rate_limit_rejected_responses'),
    },
    rawMetricNames: Object.keys(data.metrics || {}).sort(),
  };
}

export function makeHandleSummary(testName, metadataFactory) {
  return function handleSummary(data) {
    const metadata = metadataFactory();
    const summary = buildSummary(data, testName, metadata);
    const fileBase = `${safeName(testName)}-${safeName(ENV.runId)}-r${safeName(ENV.runIndex)}`;
    const jsonPath = `${ENV.resultDir}/${fileBase}.json`;
    const markdownPath = `${ENV.resultDir}/${fileBase}.md`;

    return {
      [jsonPath]: JSON.stringify(summary, null, 2),
      [markdownPath]: toMarkdown(summary),
      stdout: [
        '',
        `${testName} summary`,
        `  p50: ${markdownValue(summary.metrics.http_req_duration.med, ' ms')}`,
        `  p95: ${markdownValue(summary.metrics.http_req_duration.p95, ' ms')}`,
        `  p99: ${markdownValue(summary.metrics.http_req_duration.p99, ' ms')}`,
        `  rps: ${markdownValue(summary.metrics.http_reqs.rate)}`,
        `  failed: ${markdownValue(summary.metrics.http_req_failed.rate)}`,
        `  checks: ${markdownValue(summary.metrics.checks.rate)}`,
        '',
      ].join('\n'),
    };
  };
}
