import type { RunAgentInput } from '@ag-ui/core';
import { EventType } from '@ag-ui/core';
import type { ConnectRequest } from '@chanx-js/server';
import { PingMessage, PongMessage, consumer } from '@chanx-js/server';

import { AgUiRunTopic, agUiTopic } from '../kits/ag-ui';
import {
  BroadcastNotificationTopic,
  SubjectNotificationTopic,
  UserNotificationTopic,
} from '../kits/notification';
import { PresenceTopic } from '../kits/presence';
import { RoomChatTopic } from '../kits/room-chat';

/** What a real `authenticate` would return; the kits read `id` and `username`. */
export interface DemoUser {
  id: string;
  username: string;
}

/** The demo has no auth: `?as=<name>` stands in for it. Anonymous without it. */
function authenticate({ url }: ConnectRequest): DemoUser | '' {
  const who = url.searchParams.get('as');
  return who ? { id: who, username: who[0]!.toUpperCase() + who.slice(1) } : '';
}

export const notifications = consumer('/ws/notifications', {
  name: 'notifications',
  description: 'Fan-out notifications for the connected user.',
  authenticate,
  topics: [UserNotificationTopic, SubjectNotificationTopic, BroadcastNotificationTopic],
}).on(PingMessage, { output: PongMessage }, () => PongMessage.create());

/** Two independent kits listed side by side: chat and the roster. */
export const room = consumer('/ws/rooms/{room}', {
  name: 'room',
  description: 'Room chat with history and a live roster.',
  authenticate,
  topics: [RoomChatTopic, PresenceTopic],
}).on(PingMessage, { output: PongMessage }, () => PongMessage.create());

async function* cannedReply(input: RunAgentInput, { signal }: { signal: AbortSignal }) {
  const last = input.messages.at(-1)?.content;
  const reply =
    `You said: ${JSON.stringify(typeof last === 'string' ? last : '')}. This sandbox has no model ` +
    'credentials, so these AG-UI events are produced locally to show the protocol ' +
    'travelling over a chanx websocket.';
  const messageId = `m-${input.runId}`;
  yield { type: EventType.TEXT_MESSAGE_START, messageId, role: 'assistant' as const };
  for (const word of reply.split(' ')) {
    await new Promise((resolve) => setTimeout(resolve, 40));
    if (signal.aborted) return;
    yield { type: EventType.TEXT_MESSAGE_CONTENT, messageId, delta: `${word} ` };
  }
  yield { type: EventType.TEXT_MESSAGE_END, messageId };
}

/** AG-UI with a scripted reply: the sandbox has no model. */
export const agent = consumer('/ws/agent', {
  name: 'agent',
  description: 'AG-UI protocol over a websocket.',
  authenticate,
  topics: [agUiTopic({ runAgent: cannedReply }), AgUiRunTopic],
}).on(PingMessage, { output: PongMessage }, () => PongMessage.create());
