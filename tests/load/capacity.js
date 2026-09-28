import http from 'k6/http';
import { check, fail } from 'k6';
import exec from 'k6/execution';
import { Rate } from 'k6/metrics';

// Capacity test against a local server. Fixtures come from TestCapacity; run it
// with `make load-capacity`. See docs/report/03-scalability.md for the targets.
const base = __ENV.LOAD_BASE_URL;
if (!base || !__ENV.LOAD_FIXTURE) fail('run through TestCapacity, which seeds LOAD_FIXTURE');
const fixture = JSON.parse(open(__ENV.LOAD_FIXTURE));

const peak = Number(__ENV.CAPACITY_RPS || 300);
const hold = Number(__ENV.CAPACITY_HOLD_SECONDS || 120);
const hotRate = Number(__ENV.CAPACITY_HOT_RPS || 20);
const ramp = 60;
// k6 shares the machine with the server, so allow a sliver of undelivered load.
const planned = peak * (ramp / 2 + hold + 7.5) + hotRate * hold;

const serverErrors = new Rate('server_errors');
const orConflict = http.expectedStatuses(200, 201, 204, 409);

const reads = [
  { weight: 25, name: 'GET /events', path: () => '/events' },
  { weight: 20, name: 'GET /events/{id}', path: (id) => `/events/${id}` },
  { weight: 20, name: 'GET /events/{id}/items', path: (id) => `/events/${id}/items` },
  { weight: 20, name: 'GET /events/{id}/shares', path: (id) => `/events/${id}/shares` },
  { weight: 15, name: 'GET /events/{id}/me/details', path: (id) => `/events/${id}/me/details` },
];
const readTotal = reads.reduce((sum, r) => sum + r.weight, 0);
const writes = ['POST /events/{id}/items', 'PATCH /events/{id}/items/{item_id}'];
const perEndpoint = Object.fromEntries(
  [...reads.map((r) => r.name), ...writes].map((name) => [`http_req_duration{scenario:mixed,name:${name}}`, ['p(95)<500']]),
);

export const options = {
  scenarios: {
    // Everyday traffic: members reading, hosts saving expenses.
    mixed: {
      executor: 'ramping-arrival-rate', exec: 'mixed', startRate: 0, timeUnit: '1s',
      preAllocatedVUs: 150, maxVUs: 400,
      stages: [
        { target: peak, duration: `${ramp}s` },
        { target: peak, duration: `${hold}s` },
        { target: 0, duration: '15s' },
      ],
    },
    // Writes to one event are serialized, so one busy event queues on itself.
    hot_event: {
      executor: 'constant-arrival-rate', exec: 'hotEvent', rate: hotRate, timeUnit: '1s',
      startTime: `${ramp}s`, duration: `${hold}s`, preAllocatedVUs: 30, maxVUs: 60,
    },
    // Settle while saves race it on the same event, during peak load.
    settle: {
      executor: 'shared-iterations', exec: 'settle', iterations: fixture.settle.length, vus: 10,
      startTime: `${ramp + Math.floor(hold / 2)}s`, maxDuration: `${Math.floor(hold / 2)}s`,
    },
  },
  thresholds: {
    'http_req_duration{scenario:mixed}': ['p(95)<500'],
    'http_req_duration{scenario:hot_event}': ['p(95)<500'],
    'http_req_duration{scenario:settle}': ['p(95)<1000'],
    http_req_failed: ['rate<0.01'],
    server_errors: ['rate<0.01'],
    checks: ['rate>0.99'],
    dropped_iterations: [`count<${Math.ceil(planned * 0.005)}`],
    ...perEndpoint,
  },
};

function params(token, name, extra = {}) {
  return {
    headers: { Cookie: `session=${token}`, 'Content-Type': 'application/json' },
    tags: { name },
    ...extra,
  };
}

function pick(list) {
  return list[Math.floor(Math.random() * list.length)];
}

// Keep in step with capacityItem in tests/e2e/capacity_test.go.
function itemBody(payer) {
  return JSON.stringify({
    payer_member_id: payer,
    details: [
      { name: '肉', amount: 300 + Math.floor(Math.random() * 500), tag: '肉品' },
      { name: '飲料', amount: 120, tag: null },
    ],
  });
}

function record(res) {
  serverErrors.add(res.status >= 500);
  return res;
}


function read(event) {
  const token = Math.random() < 0.2 ? event.host : pick(event.members);
  let roll = Math.random() * readTotal;
  const r = reads.find((candidate) => (roll -= candidate.weight) < 0) || reads[0];
  const res = record(http.get(base + r.path(event.id), params(token, r.name)));
  check(res, { 'read is 200': (x) => x.status === 200 });
}

function save(event) {
  const url = `${base}/events/${event.id}/items`;
  const res = record(http.post(url, itemBody(event.payer), params(event.host, 'POST /events/{id}/items')));
  check(res, { 'save is 201': (x) => x.status === 201 });
  if (res.status !== 201 || Math.random() >= 1 / 3) return;
  const edit = JSON.stringify({ details: JSON.parse(itemBody(event.payer)).details });
  const patched = record(http.patch(`${url}/${res.json('id')}`, edit, params(event.host, 'PATCH /events/{id}/items/{item_id}')));
  check(patched, { 'edit is 200': (x) => x.status === 200 });
}

export function mixed() {
  const event = pick(fixture.events);
  if (Math.random() < 0.15) save(event);
  else read(event);
}

export function hotEvent() {
  const event = fixture.hot;
  const res = record(http.post(`${base}/events/${event.id}/items`, itemBody(event.payer), params(event.host, 'POST /events/{id}/items (hot)')));
  check(res, { 'hot save is 201': (x) => x.status === 201 });
}

export function settle() {
  const event = fixture.settle[exec.scenario.iterationInTest];
  const items = `${base}/events/${event.id}/items`;
  const racing = params(event.host, 'POST /events/{id}/items (racing settle)', { responseCallback: orConflict });
  const [settled, ...saves] = http.batch([
    ['POST', `${base}/events/${event.id}/settle`, null, params(event.host, 'POST /events/{id}/settle')],
    ['POST', items, itemBody(event.payer), racing],
    ['POST', items, itemBody(event.payer), racing],
    ['POST', items, itemBody(event.payer), racing],
  ]);
  [settled, ...saves].forEach(record);
  check(settled, { 'settle is 204': (x) => x.status === 204 });
  check(saves, { 'racing saves are 201 or 409': (xs) => xs.every((x) => x.status === 201 || x.status === 409) });
  const late = record(http.post(items, itemBody(event.payer), params(event.host, 'POST /events/{id}/items (after settle)', { responseCallback: http.expectedStatuses(409) })));
  check(late, { 'save after settle is 409': (x) => x.status === 409 });
}
