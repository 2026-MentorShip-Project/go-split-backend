import http from 'k6/http';
import { check, fail } from 'k6';

const base = __ENV.LOAD_BASE_URL;
export const options = {
  insecureSkipTLSVerify: true,
  scenarios: {
    reads: {
      executor: 'constant-arrival-rate', rate: 20, timeUnit: '1s',
      duration: '30s', preAllocatedVUs: 10, maxVUs: 30,
    },
  },
  thresholds: {
    'http_req_duration{scenario:reads}': ['p(95)<500'],
    'http_req_failed{scenario:reads}': ['rate<0.01'],
    checks: ['rate==1'],
    dropped_iterations: ['count==0'],
  },
};

export function setup() {
  const params = { headers: { 'Content-Type': 'application/json' } };
  const registered = http.post(`${base}/auth/register`, JSON.stringify({
    name: 'Load host', email: `load-${Date.now()}@example.com`, password: 'ci-password-123',
  }), params);
  if (registered.status !== 201) fail('load account setup failed');
  const created = http.post(`${base}/events`, JSON.stringify({ name: 'Load event', template: '自訂' }), params);
  if (created.status !== 201) fail('load event setup failed');
  return { token: registered.cookies.session[0].value, eventID: created.json('id') };
}

export default function (data) {
  http.cookieJar().set(base, 'session', data.token, { secure: true });
  const response = http.get(`${base}/events`);
  check(response, {
    'authenticated event list': (r) => r.status === 200,
    'created event is visible': (r) => {
      try { return r.json('events').some((event) => event.id === data.eventID && event.role === 'host'); }
      catch (_) { return false; }
    },
  });
}
