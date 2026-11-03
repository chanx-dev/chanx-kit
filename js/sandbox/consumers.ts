import type { ConnectRequest } from '@chanx-js/server';
import { PingMessage, PongMessage, consumer } from '@chanx-js/server';

import {
  BroadcastNotificationTopic,
  SubjectNotificationTopic,
  UserNotificationTopic,
} from '../notification';
import { PresenceTopic } from '../presence';
import { RoomChatTopic } from '../room-chat';

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
