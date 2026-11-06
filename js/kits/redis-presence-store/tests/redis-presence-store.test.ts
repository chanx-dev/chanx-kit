import { createClient } from 'redis';
import { describe, expect, it } from 'vitest';

import type { RedisHashClient } from '..';
import { RedisPresenceStore } from '..';

/** Enough of Redis for the store: hashes and their expiry. */
function fakeRedis() {
  const hashes = new Map<string, Map<string, string>>();
  const ttls = new Map<string, number>();
  const hash = (key: string) => hashes.get(key) ?? hashes.set(key, new Map()).get(key)!;
  const client: RedisHashClient = {
    hGet: async (key, field) => hashes.get(key)?.get(field),
    hGetAll: async (key) => Object.fromEntries(hashes.get(key) ?? []),
    hDel: async (key, field) => hash(key).delete(field),
    multi() {
      const ops: (() => void)[] = [];
      const chain: ReturnType<RedisHashClient['multi']> = {
        hSet: (key, field, value) => (ops.push(() => hash(key).set(field, value)), chain),
        expire: (key, seconds) => (ops.push(() => ttls.set(key, seconds)), chain),
        exec: async () => ops.forEach((op) => op()),
      };
      return chain;
    },
  };
  return { client, hashes, ttls };
}

// A real client fits the structural type.
export const real: () => RedisHashClient = () => createClient();

const member = (id: string) => ({ id, name: id, data: {} });

describe('redis-presence-store', () => {
  it('counts a second connection of a member once', async () => {
    const { client, ttls } = fakeRedis();
    const store = new RedisPresenceStore(client);

    expect(await store.add('lobby', 'c1', member('ann'))).toBe(true);
    expect(await store.add('lobby', 'c2', member('ann'))).toBe(false);

    expect(await store.members('lobby')).toHaveLength(1);
    expect(ttls.get('chanx-kit:presence:lobby')).toBe(3600);
  });

  it('reports the leave of the last connection only', async () => {
    const store = new RedisPresenceStore(fakeRedis().client);
    await store.add('lobby', 'c1', member('ann'));
    await store.add('lobby', 'c2', member('ann'));

    expect(await store.discard('lobby', 'c1')).toBeUndefined();
    expect(await store.discard('lobby', 'c2')).toMatchObject({ id: 'ann' });
    expect(await store.discard('lobby', 'c2')).toBeUndefined();
  });

  it('reads what the Python kit writes', async () => {
    const { client, hashes } = fakeRedis();
    hashes.set('chanx-kit:presence:lobby', new Map([['py', '{"id":"bob","name":"Bob"}']]));

    const members = await new RedisPresenceStore(client).members('lobby');

    expect(members).toEqual([{ id: 'bob', name: 'Bob', data: {} }]);
  });
});
