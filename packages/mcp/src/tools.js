import { z } from 'zod';
import { ApiError } from './client.js';

const eventId = z.number().int().positive().describe('Event id from list_events');

function ok(data) {
  return { content: [{ type: 'text', text: JSON.stringify(data, null, 2) }] };
}

function fail(error) {
  if (!(error instanceof ApiError)) throw error;
  const hint = {
    401: 'The GO_SPLIT_TOKEN is missing, expired or revoked. Mint a new one with POST /auth/tokens.',
    403: 'The signed-in user does not have the role this needs on the event.',
    409: 'The event is settled or archived, so it can no longer change.',
  }[error.status];
  return ok({ status: error.status, error: error.message, ...(hint && { hint }), ...(error.body?.details && { details: error.body.details }) });
}

function guarded(handler) {
  return async args => {
    try {
      return await handler(args);
    } catch (error) {
      return { ...fail(error), isError: true };
    }
  };
}

export function registerTools(server, request) {
  server.registerTool('list_events', {
    title: 'List events',
    description: 'List every Go-Split event the user belongs to, newest first, with their role (host, co or member) and whether it is settled or archived.',
    annotations: { readOnlyHint: true, openWorldHint: true },
  }, guarded(async () => ok((await request('GET', '/events')).events)));

  server.registerTool('get_balances', {
    title: 'Get balances',
    description: 'Show who owes what in an event. Returns members (use their ids for add_expense), each member\'s owed, advanced and net in whole NT dollars (net > 0 receives, net < 0 pays), and the transfers that settle the event. Ordinary members only see their own share and no transfers until the event is archived. Before settlement these are previews.',
    inputSchema: { event_id: eventId },
    annotations: { readOnlyHint: true, openWorldHint: true },
  }, guarded(async ({ event_id }) => {
    const [event, shares, transfers] = await Promise.all([
      request('GET', `/events/${event_id}`),
      request('GET', `/events/${event_id}/shares`),
      request('GET', `/events/${event_id}/transfers`).catch(error => {
        if (error instanceof ApiError && error.status === 403) return null;
        throw error;
      }),
    ]);
    const name = id => event.members.find(m => m.id === id)?.display ?? `member ${id}`;
    return ok({
      event: { id: event.id, name: event.name, settled: event.settled, archived: event.archived },
      members: event.members.map(m => ({ id: m.id, name: m.display, role: m.role, you: m.you ?? false })),
      grand_total: shares.grand_total,
      balances: shares.per_member.map(s => ({ member_id: s.member_id, name: name(s.member_id), owed: s.owed, advanced: s.advanced, net: s.net })),
      transfers: transfers?.transfers.map(t => ({ from: name(t.from_id), to: name(t.to_id), amount: t.amount })) ?? 'only visible to the host until the event is archived',
    });
  }));

  server.registerTool('add_expense', {
    title: 'Add expense',
    description: 'Add an expense card paid by one member, with one or more line items. Amounts are whole NT dollars. Each line is split by the event\'s tag rules, or only among member_ids when given. Needs host or co role.',
    inputSchema: {
      event_id: eventId,
      payer_member_id: z.number().int().positive().describe('Member id of who paid, from get_balances'),
      has_receipt: z.boolean().optional(),
      details: z.array(z.object({
        name: z.string().min(1).max(120).describe('What was bought'),
        amount: z.number().int().positive().describe('Whole NT dollars'),
        tag: z.string().optional().describe('Item tag from the event catalog; drives the split rules'),
        note: z.string().optional(),
        member_ids: z.array(z.number().int().positive()).optional().describe('Split only among these members'),
      })).min(1),
    },
    annotations: { readOnlyHint: false, destructiveHint: false, idempotentHint: false, openWorldHint: true },
  }, guarded(async ({ event_id, payer_member_id, has_receipt = false, details }) => {
    const item = await request('POST', `/events/${event_id}/items`, {
      payer_member_id,
      has_receipt,
      details: details.map(d => ({
        name: d.name,
        amount: d.amount,
        tag: d.tag ?? null,
        note: d.note ?? '',
        custom_amounts: {},
        manual_member_ids: d.member_ids ?? [],
      })),
    });
    return ok({ item_id: item.id, total: item.total, details: item.details.map(d => ({ id: d.id, name: d.name, amount: d.amount })) });
  }));

  server.registerTool('settle_up', {
    title: 'Settle event',
    description: 'Permanently freeze an event: validates every split, locks all edits and invitations, and stores the final transfers. This cannot be undone. Host only. Only call after the user has seen get_balances and explicitly asked to settle.',
    inputSchema: { event_id: eventId },
    annotations: { readOnlyHint: false, destructiveHint: true, idempotentHint: false, openWorldHint: true },
  }, guarded(async ({ event_id }) => {
    await request('POST', `/events/${event_id}/settle`);
    return ok({ settled: true, next: 'Call get_balances to show the final transfers.' });
  }));
}
