import http from 'k6/http';
import { check, fail } from 'k6';

// Measures what the Next.js proxy adds. Each iteration sends the same read both
// straight to the backend and through the frontend's /api rewrite, in random
// order, so network drift hits both paths alike. Low rate: this is a timing
// measurement, not a load test. Run it with `make load-hop`.
const direct = __ENV.LOAD_BACKEND_URL;
const proxied = __ENV.LOAD_FRONTEND_URL;
const session = __ENV.LOAD_SESSION;
const eventID = __ENV.LOAD_EVENT_ID;
if (!direct || !proxied || !session || !eventID) fail('run through tests/load/run-hop.sh');

export const options = {
  scenarios: {
    hop: {
      executor: 'constant-arrival-rate', rate: Number(__ENV.HOP_RPS || 5), timeUnit: '1s',
      duration: `${Number(__ENV.HOP_SECONDS || 60)}s`, preAllocatedVUs: 5, maxVUs: 20,
    },
  },
  // Listed so k6 reports each path separately; the comparison is in handleSummary.
  thresholds: {
    'http_req_duration{route:direct}': ['max>=0'],
    'http_req_duration{route:proxied}': ['max>=0'],
    http_req_failed: ['rate<0.01'],
  },
};

const reads = [
  { name: 'GET /events', path: '/events' },
  { name: 'GET /events/{id}', path: `/events/${eventID}` },
];

export default function () {
  const read = reads[Math.floor(Math.random() * reads.length)];
  const routes = [['direct', direct], ['proxied', proxied]];
  if (Math.random() < 0.5) routes.reverse();
  for (const [route, base] of routes) {
    const res = http.get(base + read.path, { headers: { Cookie: `session=${session}` }, tags: { route, name: read.name } });
    check(res, { [`${route} is 200`]: (r) => r.status === 200 });
  }
}

export function handleSummary(data) {
  const stat = (route, key) => data.metrics[`http_req_duration{route:${route}}`].values[key];
  const rows = ['med', 'p(90)', 'p(95)'].map((key) => {
    const d = stat('direct', key);
    const p = stat('proxied', key);
    return `${key.padEnd(6)} ${d.toFixed(1).padStart(9)} ms ${p.toFixed(1).padStart(9)} ms ${(p - d).toFixed(1).padStart(9)} ms`;
  });
  const failed = (data.metrics.http_req_failed.values.rate * 100).toFixed(2);
  const text = [
    '',
    `Proxy hop: ${direct}  vs  ${proxied}`,
    `${''.padEnd(6)} ${'direct'.padStart(12)} ${'proxied'.padStart(12)} ${'added'.padStart(12)}`,
    ...rows,
    `requests: ${data.metrics.http_reqs.values.count}, failed: ${failed}%`,
    '',
  ].join('\n');
  return { stdout: text, 'artifacts/hop-summary.json': JSON.stringify(data, null, 2) };
}
