# redis-presence-store

Shared presence roster backed by Redis, so presence stays correct across several
processes.

```bash
copit add @chanx-kit-js/redis-presence-store   # also installs: presence
```

## Why

The `presence` kit defaults to an in-memory store, which keeps the roster in one
process. Run two replicas and each reports only the connections it holds. This store
puts that state in Redis.

## Use it

```ts
import { createClient } from 'redis';

import { presenceTopic } from './ws-kits/presence';
import { RedisPresenceStore } from './ws-kits/redis-presence-store';

const redis = await createClient({ url: process.env.REDIS_URL }).connect();

export const PresenceTopic = presenceTopic({ store: new RedisPresenceStore(redis) });
```

Any client with node-redis's `hGet`, `hGetAll`, `hDel` and `multi` fits.

## How state is kept

One hash per scope, `chanx-kit:presence:<scope>`, mapping connection id to the member
JSON, as the Python kit keeps it: Python, Go and Node servers sharing a Redis share one
roster. Membership is derived from the hash, so a process that dies cannot leave a
counter wrong, and each write refreshes a TTL (an hour by default) so an abandoned scope
expires.

```ts
new RedisPresenceStore(redis, { keyPrefix: 'myapp:presence', ttl: 1800 });
```

## Trade-offs

- Every join and leave costs a round trip; `members()` costs one `HGETALL`.
- The TTL is a backstop, not a heartbeat: lower it and re-register connections if you
  need faster cleanup after a crash.
- `add()` and `discard()` read then write, so one member's tabs churning on several
  processes at the same instant can duplicate or skip an announcement. The roster
  heals on the next read.
