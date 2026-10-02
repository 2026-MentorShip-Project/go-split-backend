import assert from 'node:assert/strict';
import { test } from 'node:test';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { InMemoryTransport } from '@modelcontextprotocol/sdk/inMemory.js';
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { createClient } from '../src/client.js';
import { registerTools } from '../src/tools.js';

function fakeApi(routes) {
  const calls = [];
  const fetch = async (url, init) => {
    const path = new URL(url).pathname;
    const key = `${init.method} ${path}`;
    calls.push({ key, headers: init.headers, body: init.body && JSON.parse(init.body) });
    const [status, body] = routes[key] ?? [404, { error: 'not found' }];
    return new Response(body === undefined ? '' : JSON.stringify(body), { status });
  };
  return { calls, fetch };
}

async function connect(routes) {
  const api = fakeApi(routes);
  const server = new McpServer({ name: 'go-split', version: 'test' });
  registerTools(server, createClient({ baseUrl: 'https://api.test/', token: 'tok', fetch: api.fetch }));
  const client = new Client({ name: 'test', version: 'test' });
  const [a, b] = InMemoryTransport.createLinkedPair();
  await Promise.all([server.connect(a), client.connect(b)]);
  const call = async (name, args = {}) => {
    const result = await client.callTool({ name, arguments: args });
    return { ...result, data: JSON.parse(result.content[0].text) };
  };
  return { api, client, call };
}

const event = {
  id: 7, name: 'BBQ', settled: false, archived: false,
  members: [{ id: 1, display: 'Ann', role: 'host', you: true }, { id: 2, display: 'Ben', role: 'member' }],
};
const shares = {
  grand_total: 300,
  per_member: [{ member_id: 1, owed: 150, advanced: 300, net: 150 }, { member_id: 2, owed: 150, advanced: 0, net: -150 }],
};

test('given the server when listing tools then settle_up alone is destructive', async () => {
  const { client } = await connect({});
  const { tools } = await client.listTools();
  assert.deepEqual(tools.map(t => t.name).sort(), ['add_expense', 'get_balances', 'list_events', 'settle_up']);
  assert.deepEqual(tools.filter(t => t.annotations?.destructiveHint).map(t => t.name), ['settle_up']);
});

test('given a token when listing events then it sends a bearer header and returns the events', async () => {
  const { api, call } = await connect({ 'GET /events': [200, { events: [{ id: 7, name: 'BBQ' }] }] });
  const { data } = await call('list_events');
  assert.deepEqual(data, [{ id: 7, name: 'BBQ' }]);
  assert.equal(api.calls[0].headers.Authorization, 'Bearer tok');
});

test('given a host when getting balances then transfers carry member names', async () => {
  const { call } = await connect({
    'GET /events/7': [200, event],
    'GET /events/7/shares': [200, shares],
    'GET /events/7/transfers': [200, { transfers: [{ from_id: 2, to_id: 1, amount: 150 }] }],
  });
  const { data } = await call('get_balances', { event_id: 7 });
  assert.equal(data.balances[1].name, 'Ben');
  assert.deepEqual(data.transfers, [{ from: 'Ben', to: 'Ann', amount: 150 }]);
});

test('given a member when getting balances then the hidden transfers do not fail the call', async () => {
  const { call } = await connect({
    'GET /events/7': [200, event],
    'GET /events/7/shares': [200, shares],
    'GET /events/7/transfers': [403, { error: 'host only' }],
  });
  const result = await call('get_balances', { event_id: 7 });
  assert.equal(result.isError, undefined);
  assert.match(result.data.transfers, /only visible to the host/);
});

test('given line items when adding an expense then it posts the backend shape', async () => {
  const { api, call } = await connect({
    'POST /events/7/items': [201, { id: 9, total: 300, details: [{ id: 11, name: 'Meat', amount: 300 }] }],
  });
  const { data } = await call('add_expense', {
    event_id: 7, payer_member_id: 1, details: [{ name: 'Meat', amount: 300, member_ids: [1, 2] }],
  });
  assert.equal(data.item_id, 9);
  assert.deepEqual(api.calls[0].body, {
    payer_member_id: 1, has_receipt: false,
    details: [{ name: 'Meat', amount: 300, tag: null, note: '', custom_amounts: {}, manual_member_ids: [1, 2] }],
  });
});

test('given invalid details when adding an expense then the tool error carries the issues', async () => {
  const issues = [{ index: 0, code: 'unknown-item-tag' }];
  const { call } = await connect({ 'POST /events/7/items': [422, { error: 'invalid details', details: issues }] });
  const result = await call('add_expense', { event_id: 7, payer_member_id: 1, details: [{ name: 'X', amount: 1, tag: 'nope' }] });
  assert.equal(result.isError, true);
  assert.deepEqual(result.data.details, issues);
});

test('given a settled event when settling then the tool explains the 409', async () => {
  const { call } = await connect({ 'POST /events/7/settle': [409, { error: 'event is settled' }] });
  const result = await call('settle_up', { event_id: 7 });
  assert.equal(result.isError, true);
  assert.match(result.data.hint, /settled or archived/);
});

test('given an expired token then the tool says how to get a new one', async () => {
  const { call } = await connect({ 'GET /events': [401, { error: 'not signed in' }] });
  const result = await call('list_events');
  assert.equal(result.isError, true);
  assert.match(result.data.hint, /POST \/auth\/tokens/);
});
