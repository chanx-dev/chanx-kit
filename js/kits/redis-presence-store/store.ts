import type { PresenceMember, PresenceStore } from '../presence';

/** The node-redis calls this store makes; a `createClient()` client fits. */
export interface RedisHashClient {
  hGet(key: string, field: string): Promise<unknown>;
  hGetAll(key: string): Promise<unknown>;
  hDel(key: string, field: string): Promise<unknown>;
  multi(): RedisHashMulti;
}

export interface RedisHashMulti {
  hSet(key: string, field: string, value: string): RedisHashMulti;
  expire(key: string, seconds: number): RedisHashMulti;
  exec(): Promise<unknown>;
}

export interface RedisPresenceStoreOptions {
  /** `chanx-kit:presence` by default, as the Python kit. */
  keyPrefix?: string;
  /** Seconds a roster survives with no writes; every write refreshes it. An hour by default. */
  ttl?: number;
}

/**
 * The presence roster in Redis: one hash per scope of connection id to member JSON,
 * laid out as the Python kit lays it out, so Python, Go and Node servers share it.
 */
export class RedisPresenceStore implements PresenceStore {
  private readonly keyPrefix: string;
  private readonly ttl: number;

  constructor(
    private readonly redis: RedisHashClient,
    { keyPrefix = 'chanx-kit:presence', ttl = 3600 }: RedisPresenceStoreOptions = {},
  ) {
    this.keyPrefix = keyPrefix;
    this.ttl = ttl;
  }

  private key(scope: string) {
    return `${this.keyPrefix}:${scope}`;
  }

  async add(scope: string, connectionId: string, member: PresenceMember) {
    const key = this.key(scope);
    const present = (await this.connections(key)).some((m) => m.id === member.id);
    await this.redis
      .multi()
      .hSet(key, connectionId, JSON.stringify(member))
      .expire(key, this.ttl)
      .exec();
    return !present;
  }

  async discard(scope: string, connectionId: string) {
    const key = this.key(scope);
    const raw = await this.redis.hGet(key, connectionId);
    if (typeof raw !== 'string') return undefined;
    const member = JSON.parse(raw) as PresenceMember;
    await this.redis.hDel(key, connectionId);
    const remaining = await this.connections(key);
    return remaining.some((m) => m.id === member.id) ? undefined : member;
  }

  async members(scope: string) {
    const seen = new Map<string, PresenceMember>();
    for (const member of await this.connections(this.key(scope))) {
      if (!seen.has(member.id)) seen.set(member.id, member);
    }
    return [...seen.values()];
  }

  private async connections(key: string): Promise<PresenceMember[]> {
    const hash = (await this.redis.hGetAll(key)) as Record<string, string>;
    return Object.values(hash).map((raw) => ({ data: {}, ...JSON.parse(raw) }) as PresenceMember);
  }
}
