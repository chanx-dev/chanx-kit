# presence

Track who is present in a room, document or tenant, and tell everyone when that changes.
Speaks **presence@1**, like the Python kit, so the `presence` UI kit works unchanged.

```bash
copit add @chanx-kit-js/presence
```

## Use it

List the topic on a consumer:

```ts
import { PresenceTopic } from './ws-kits/presence';

const room = consumer('/ws/rooms/{room}', { name: 'room', authenticate, topics: [PresenceTopic] });
```

A client subscribes to `presence:<scope>` and receives `presence_state` with the full
roster once, then `presence_join` / `presence_leave` as it changes. Join and leave reach
the whole scope, the member who triggered them included.

Presence is keyed on the member, not the connection: three tabs of one user produce one
join, and the leave fires when the last one closes.

## Customise

```ts
const DocumentPresence = presenceTopic<User>({
  store: redisStore,
  member: ({ user }) => ({ id: user.id, name: user.fullName, data: { avatar: user.avatarUrl } }),
});

await DocumentPresence.membersOf('doc-42');
```

| Option | Default | Purpose |
|---|---|---|
| `member(conn)` | `memberOf`: a string user, or its `id` and `username` | Who is present |
| `store` | `InMemoryPresenceStore` | Where the roster lives |
| `name` | `PresenceTopic` | Group namespace, shared with servers on the same layer |

## Production: replace the store

`InMemoryPresenceStore` is per process: with two replicas each sees half the room.
Implement `PresenceStore` against shared storage.

## Messages

| Action | Direction | Payload |
|---|---|---|
| `presence_state` | server → client | `scope`, `members` |
| `presence_join` | server → client | `scope`, `member` |
| `presence_leave` | server → client | `scope`, `member` |
| `presence_request` | client → server | none |
