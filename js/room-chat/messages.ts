import { randomUUID } from 'node:crypto';

import { defineMessage } from '@chanx-js/server';
import { z } from 'zod';

export const ChatAuthor = z
  .object({ id: z.string(), name: z.string().nullish() })
  .meta({ id: 'ChatAuthor', title: 'ChatAuthor' });

export type ChatAuthor = z.input<typeof ChatAuthor>;

export const ChatEntry = z
  .object({
    body: z.string(),
    author: ChatAuthor,
    room: z.string(),
    id: z.string().optional(),
    sent_at: z.iso.datetime().optional(),
  })
  .meta({ id: 'ChatEntry', title: 'ChatEntry' });

export type ChatEntry = Required<z.output<typeof ChatEntry>>;

/** An entry with its id and time filled. */
export function chatEntry(room: string, body: string, author: ChatAuthor): ChatEntry {
  return {
    body,
    author,
    room,
    id: randomUUID().replaceAll('-', ''),
    sent_at: new Date().toISOString(),
  };
}

export const ChatSendPayload = z.object({ body: z.string() }).meta({ title: 'ChatSendPayload' });

export const ChatBacklogPayload = z
  .object({ room: z.string(), entries: z.array(ChatEntry) })
  .meta({ title: 'ChatBacklogPayload' });

export const ChatBacklogRequestPayload = z
  .object({ limit: z.number().int().nullish() })
  .meta({ title: 'ChatBacklogRequestPayload' });

export const ChatSendMessage = defineMessage('chat_send', ChatSendPayload, {
  description: 'Client posts a message to the room.',
});

export const ChatMessage = defineMessage('chat_message', ChatEntry, {
  title: 'ChatMessage',
  description: 'A message published to the room.',
});

export const ChatBacklogMessage = defineMessage('chat_backlog', ChatBacklogPayload, {
  description: 'Recent history, sent to a connection when it joins.',
});

export const ChatBacklogRequestMessage = defineMessage(
  'chat_backlog_request',
  ChatBacklogRequestPayload.default({}),
  { description: 'Client asks for history again, without reconnecting.' },
);
