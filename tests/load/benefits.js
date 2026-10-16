// D13 / HU-29 / HU-32 / RN-22 / RN-26 / RN-27 / CL-08b.
// Datos y credencial efimeros: el runner los entrega fuera del repositorio.
import http from 'k6/http';
import { sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { SharedArray } from 'k6/data';
import { sha256 } from 'k6/crypto';
import exec from 'k6/execution';

const accounts = new SharedArray('accounts', () => JSON.parse(open('/input/fixture.json')).accounts);
const kind = __ENV.WORKLOAD;
const distribution = __ENV.DISTRIBUTION;
const vus = Number(__ENV.VUS);
const pause = Number(__ENV.PAUSE_MS) / 1000;
const failures = new Rate('functional_failures');
const reads = new Trend('read_ms', true);
const writes = new Trend('write_ms', true);
const replays = new Trend('replay_ms', true);
const created = new Counter('credits_created');
const replayed = new Counter('credits_replayed');
const readsCount = new Counter('reads_completed');

export const options = {
  scenarios: { load: { executor: 'constant-vus', vus, duration: __ENV.DURATION } },
  systemTags: ['method', 'status', 'name', 'scenario', 'expected_response'],
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(50)', 'p(95)', 'p(99)'],
  thresholds: {
    functional_failures: ['rate==0'],
    ...(kind === 'read' || kind === 'mixed' ? { read_ms: ['p(95)<200'] } : {}),
    ...(kind === 'write' || kind === 'mixed' ? { write_ms: ['p(95)<500'] } : {}),
    ...(kind === 'replay' ? { replay_ms: ['p(95)<500'] } : {}),
  },
};

export default function () {
  const iteration = exec.scenario.iterationInTest;
  const slot = exec.vu.idInTest - 1;
  const index = distribution === 'hot' ? 0 : (iteration + slot) % accounts.length;
  const account = accounts[index];
  const operation = kind === 'mixed' ? (iteration % 5 === 0 ? 'write' : 'read') : kind;
  const headers = { 'X-API-Key': __ENV.GYM_LOAD_API_KEY, 'Content-Type': 'application/json' };
  let response;
  let valid;
  try {
  if (operation === 'read') {
    const balance = iteration % 2 === 0;
    const suffix = balance ? '/balance' : '/movements?page=1&size=20';
    response = http.get(`${__ENV.BASE_URL}/partner-api/v1/accounts/${account.externalUser}${suffix}`, { headers, tags: { name: balance ? 'balance' : 'movements' } });
    reads.add(response.timings.duration);
    valid = response.status === 200;
    if (valid) {
      const data = response.json();
      valid = balance ? data.externalUserId === account.externalUser && Number.isInteger(data.balance) && data.balance >= 0 : Array.isArray(data.items) && data.totalItems >= 0;
    }
    if (valid) readsCount.add(1);
  } else {
    const replay = operation === 'replay';
    headers['Idempotency-Key'] = replay ? account.seeds[slot].key : `${__ENV.RUN_ID}-${exec.vu.idInTest}-${iteration}`;
    response = http.post(`${__ENV.BASE_URL}/partner-api/v1/accounts/${account.externalUser}/credits`, JSON.stringify({ points: 1, reason: 'Carga sintetica', reference: 'load-fixture' }), { headers, tags: { name: replay ? 'replay' : 'credit' } });
    (replay ? replays : writes).add(response.timings.duration);
    valid = response.status === 201;
    if (replay) {
      valid = valid && response.headers['Idempotent-Replayed'] === 'true' && sha256(response.body, 'hex') === account.seeds[slot].hash;
      if (valid) replayed.add(1);
    } else {
      valid = valid && response.headers['Idempotent-Replayed'] === 'false';
      if (valid) created.add(1);
    }
  }
  } catch (_) { valid = false; }
  failures.add(!valid);
  sleep(pause);
}

export function handleSummary(data) {
  // Solo agregados; no guardar IDs, cuerpos, claves ni URLs concretas.
  const summary = {
    workload: kind, distribution, vus, pause_ms: Number(__ENV.PAUSE_MS),
    duration: __ENV.DURATION, phase: __ENV.PHASE,
    metrics: data.metrics, state: data.state,
  };
  return { '/output/summary.json': JSON.stringify(summary, null, 2) };
}
