import type { Connection } from '@chanx-js/server';
import { topic } from '@chanx-js/server';

import type { PresenceMemberInput } from './messages';
import {
  PresenceMember,
  PresenceJoinMessage,
  PresenceLeaveMessage,
  PresenceRequestMessage,
  PresenceStateMessage,
} from './messages';
import type { PresenceStore } from './store';
import { InMemoryPresenceStore } from './store';

export interface PresenceTopicOptions<User> {
  /** Where the roster lives; in memory by default. */
  store?: PresenceStore;
  /** Who this connection is; by default from its user, else anonymous. */
  member?: (conn: Connection<string, User>) => PresenceMemberInput;
  /** Group namespace, shared with any server on the same layer. */
  name?: string;
}

/** A string user, or one with `id` and `username` or `name`; anonymous otherwise. */
export function memberOf(conn: Connection<string, unknown>): PresenceMemberInput {
  const user = conn.user as
    | string
    | { id?: unknown; username?: unknown; name?: unknown }
    | null
    | undefined;
  if (typeof user === 'string' && user) return { id: user, name: user };
  if (typeof user === 'object' && user?.id != null) {
    const id = String(user.id);
    return { id, name: String(user.username ?? user.name ?? id) };
  }
  return { id: conn.channelName, name: 'anonymous' };
}

/** `presence:<scope>`: the roster on subscribe, then joins and leaves, once per member. */
export function presenceTopic<User = unknown>(options: PresenceTopicOptions<User> = {}) {
  const store = options.store ?? new InMemoryPresenceStore();
  const member = options.member ?? memberOf;
  const state = async (scope: string) =>
    PresenceStateMessage.create({ scope, members: await store.members(scope) });

  const built = topic('presence:{scope}', {
    name: options.name ?? 'PresenceTopic',
    description: 'Who is present in a scope.',
    onSubscribe: async (ctx) => {
      const { scope } = ctx.params;
      const who = PresenceMember.parse(member(ctx.conn as Connection<string, User>));
      const joined = await store.add(scope, ctx.conn.channelName, who);
      await ctx.push(await state(scope));
      if (joined) await ctx.publish(PresenceJoinMessage.create({ scope, member: who }));
    },
    onUnsubscribe: async (ctx) => {
      const { scope } = ctx.params;
      const departed = await store.discard(scope, ctx.conn.channelName);
      if (departed) await ctx.publish(PresenceLeaveMessage.create({ scope, member: departed }));
    },
  })
    .on(
      PresenceRequestMessage,
      {
        output: PresenceStateMessage,
        summary: 'Request the roster',
        description: 'Return who is currently present, without reconnecting.',
      },
      (_m, ctx) => state(ctx.params.scope),
    )
    .passthrough(PresenceJoinMessage, PresenceLeaveMessage);

  return Object.assign(built, {
    /** Who is present in `scope`, callable from anywhere. */
    membersOf: (scope: string) => store.members(scope),
  });
}

export const PresenceTopic = presenceTopic();
