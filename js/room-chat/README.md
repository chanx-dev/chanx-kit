# room-chat

A chat room: persisted history replayed on connect, plus a live roster. Speaks
**chat@1**, like the Python kit, so the `chat` UI kit works unchanged.

```bash
copit add @chanx-kit-js/room-chat   # installs presence too
```

## Use it

List the topic on a consumer, next to presence for the roster:

```ts
import { PresenceTopic } from './ws-kits/presence';
import { RoomChatTopic } from './ws-kits/room-chat';

const room = consumer('/ws/rooms/{room}', {
  name: 'room',
  authenticate,
  topics: [RoomChatTopic, PresenceTopic],
});
```

A client subscribes to `chat:<room>` and receives `chat_backlog` with recent history,
then `chat_message` for anything posted afterwards. Posting persists before it
publishes, so a message that fails to save is never shown as delivered.

## Post without a connection

```ts
await RoomChatTopic.postToRoom(chanx, 'general', 'deploy finished', { id: 'bot' });
const entries = await RoomChatTopic.history('general', 20);
```

## Customise

```ts
const Chat = roomChatTopic<User>({
  store: new PostgresMessageStore(db),
  backlogLimit: 100,
  author: ({ user }) => ({ id: user.id, name: user.displayName }),
});
```

| Option | Default | Purpose |
|---|---|---|
| `author(conn)` | `authorOf`: a string user, or its `id` and `username` | Who the connection posts as |
| `store` | `InMemoryMessageStore` | Where history lives |
| `backlogLimit` | `50` | How much history a joiner gets |
| `name` | `RoomChatTopic` | Group namespace, shared with servers on the same layer |

## Production: replace the store

`InMemoryMessageStore` is bounded and per process: history is lost on restart and
differs between replicas. Implement `MessageStore` against your database.

## Messages

| Action | Direction | Payload |
|---|---|---|
| `chat_send` | client → server | `body` |
| `chat_message` | server → client | `body`, `author`, `room`, `id`, `sent_at` |
| `chat_backlog` | server → client | `room`, `entries` |
| `chat_backlog_request` | client → server | `limit` |
