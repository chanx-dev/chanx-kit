import type { Chanx, Connection } from '@chanx-js/server';
import { topic } from '@chanx-js/server';

import type { ChatAuthor } from './messages';
import {
  ChatBacklogMessage,
  ChatBacklogRequestMessage,
  ChatMessage,
  ChatSendMessage,
  chatEntry,
} from './messages';
import type { MessageStore } from './store';
import { InMemoryMessageStore } from './store';

export interface RoomChatTopicOptions<User> {
  /** Where history lives; in memory, and not durable, by default. */
  store?: MessageStore;
  /** How much history a joiner gets; 50 by default. */
  backlogLimit?: number;
  /** Who this connection posts as; by default from its user, else anonymous. */
  author?: (conn: Connection<string, User>) => ChatAuthor;
  /** Group namespace, shared with any server on the same layer. */
  name?: string;
}

/** A string user, or one with `id` and `username` or `name`; anonymous otherwise. */
export function authorOf(conn: Connection<string, unknown>): ChatAuthor {
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

/** Chat for `chat:<room>`: `chat_backlog` on subscribe, then `chat_message` as they arrive. */
export function roomChatTopic<User = unknown>(options: RoomChatTopicOptions<User> = {}) {
  const store = options.store ?? new InMemoryMessageStore();
  const author = options.author ?? authorOf;
  const backlog = async (room: string, limit?: number | null) =>
    ChatBacklogMessage.create({
      room,
      entries: await store.backlog(room, limit || (options.backlogLimit ?? 50)),
    });

  const built = topic('chat:{room}', {
    name: options.name ?? 'RoomChatTopic',
    description: 'Chat for one room.',
    onSubscribe: async (ctx) => ctx.push(await backlog(ctx.params.room)),
  })
    .on(
      ChatBacklogRequestMessage,
      {
        output: ChatBacklogMessage,
        summary: 'Request history',
        description: 'Return recent messages again, without reconnecting.',
      },
      (m, ctx) => backlog(ctx.params.room, m.payload.limit),
    )
    .on(
      ChatSendMessage,
      {
        output: ChatMessage,
        summary: 'Post a chat message',
        description: 'Persist a message and publish it to everyone in the room.',
      },
      // Persist first, publish second: an unsaved message is never shown as delivered.
      async (m, ctx) => {
        const entry = chatEntry(
          ctx.params.room,
          m.payload.body,
          author(ctx.conn as Connection<string, User>),
        );
        await store.append(entry);
        await ctx.publish(ChatMessage.create(entry));
      },
    )
    .passthrough(ChatMessage);

  return Object.assign(built, {
    /** Persist and deliver a message to `room`, with no connection involved. */
    async postToRoom(chanx: Chanx, room: string, body: string, by: ChatAuthor) {
      const entry = chatEntry(room, body, by);
      await store.append(entry);
      await chanx.publish(built, built.with({ room }), ChatMessage.create(entry));
      return entry;
    },
    /** Recent messages for `room`, callable from anywhere. */
    history: (room: string, limit = 50) => store.backlog(room, limit),
  });
}

export const RoomChatTopic = roomChatTopic();
