# notification

Fan out notifications to a user's live connections from anywhere in your app: a request
handler, a background worker, another service. Speaks **notification@1**, like the Python
kit, so the `notification` UI kit works unchanged.

```bash
copit add @chanx-kit-js/notification
```

## Use it

List the topics on a consumer:

```ts
import { consumer } from '@chanx-js/server';

import {
  BroadcastNotificationTopic,
  UserNotificationTopic,
} from './ws-kits/notification';

const app = consumer('/ws/notifications', {
  name: 'notifications',
  authenticate: async (request) => (await userFrom(request)) ?? false,
  topics: [UserNotificationTopic, BroadcastNotificationTopic],
});
```

A client subscribes to the audiences it wants (`notification:user:<id>`,
`notification:subject:<name>`, `notification:all`). Asking for another user's
notifications is refused: the user id is what `authenticate` returned, when it is a
string, or its `id`.

## Send a notification

From anywhere with the app:

```ts
import { notifyAll, notifyUser } from './ws-kits/notification';

await notifyUser(chanx, user.id, { title: 'Build finished', level: 'success' });
await notifyAll(chanx, { title: 'Maintenance at 22:00' });
```

`id`, `created_at`, `level` and `data` are filled when left out.

## Customise

Build the topic with options, or edit the copied code:

```ts
const UserTopic = userNotificationTopic<User>({
  userId: (user) => user.accountId,
  onAcked: (ids) => store.markRead(ids),
});

await notifyUser(chanx, user.accountId, { title: 'Hi' }, UserTopic);
```

| Option | Default | Purpose |
|---|---|---|
| `userId(user)` | the user, or its `id` | Whose notifications this connection gets |
| `onAcked(ids, ctx)` | none | Persist read state |
| `name` | `UserNotificationTopic` | Group namespace, shared with servers on the same layer |

## Messages

| Action | Direction | Payload |
|---|---|---|
| `notification` | server → client | `title`, `body`, `level`, `id`, `created_at`, `data` |
| `notification_ack` | client → server | `ids` |
| `notification_acked` | server → client | `ids` |
