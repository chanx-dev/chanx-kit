// WebSocket consumers. After copit add, list the kit's topics here.
import type { ChannelLayer } from '@chanx-js/layers';
import { InMemoryChannelLayer } from '@chanx-js/layers';
import { RedisPubSubChannelLayer } from '@chanx-js/layers/redis';
import type { ConnectRequest } from '@chanx-js/server';
import { PingMessage, PongMessage, consumer, createChanx } from '@chanx-js/server';

import {
  BroadcastNotificationTopic,
  UserNotificationTopic,
  notifyAll,
  notifyUser,
} from './ws-kits/notification';

/** Demo identity from `?as=<name>`, the user the kits read. Replace with your auth. */
function authenticate({ url }: ConnectRequest): { id: string } | '' {
  const who = url.searchParams.get('as');
  return who ? { id: who } : '';
}

export const notifications = consumer('/ws/notifications', {
  name: 'notifications',
  description: 'Notifications for the connected user, and for everyone.',
  authenticate,
  topics: [UserNotificationTopic, BroadcastNotificationTopic],
}).on(PingMessage, { output: PongMessage }, () => PongMessage.create());

// Redis when REDIS_URL is set (needed across processes), in memory otherwise.
const layer: ChannelLayer = process.env.REDIS_URL
  ? new RedisPubSubChannelLayer({ hosts: [process.env.REDIS_URL], prefix: 'chanx-app' })
  : new InMemoryChannelLayer();

export const chanx = createChanx({ consumers: [notifications], layers: { default: layer } });

export const asyncapi = {
  title: 'chanx app',
  version: '0.1.0',
  description: 'WebSocket API of this app.',
};

/** One user's connections, or everyone when `user` is empty. */
export function notify(title: string, user?: string | null) {
  return user ? notifyUser(chanx, user, { title }) : notifyAll(chanx, { title });
}
