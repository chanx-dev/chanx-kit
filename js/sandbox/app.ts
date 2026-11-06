import type { ChannelLayer } from '@chanx-js/layers';
import { InMemoryChannelLayer } from '@chanx-js/layers';
import { RedisPubSubChannelLayer } from '@chanx-js/layers/redis';
import { createChanx } from '@chanx-js/server';

import { agent, notifications, room } from './consumers';

/** REDIS_URL from the environment or the repo's `.env`. */
export function redisUrl(): string | undefined {
  try {
    process.loadEnvFile(new URL('../../.env', import.meta.url));
  } catch {
    // No .env: the in-memory layer.
  }
  return process.env.REDIS_URL || undefined;
}

/** Redis on the Python sandbox's prefix, so either server reaches the other's clients. */
export function createSandbox(redis?: string) {
  const layer: ChannelLayer = redis
    ? new RedisPubSubChannelLayer({ hosts: [redis], prefix: 'chanx-kit' })
    : new InMemoryChannelLayer();
  return createChanx({ consumers: [notifications, room, agent], layers: { default: layer } });
}

export const asyncapi = {
  title: 'chanx-kit sandbox',
  version: '0.1.0',
  description: 'WebSocket API exposed by the chanx-kit JS sandbox.',
};
