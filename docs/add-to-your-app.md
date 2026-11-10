---
description: "Set up the kit registries, add a kit, mount it on your server and show it in the browser."
---

# Add kits to your app

For a project you already have. Starting fresh is quicker with
[a template](start-a-project.md).

!!! note "Before you start"
    Your server needs chanx and a consumer, and your React app the chanx client.
    [Set up an existing app](set-up-your-app.md) shows both halves for each framework.

## 1. Set up the registries

Kits are copied in with [copit](https://github.com/huynguyengl99/copit). Install it once:

--8<-- "docs/.snippets/install-copit.md"

Then tell it, once per project, where kits come from: your server's registry and the UI
one. It writes this down in `copit.toml`, so later commands need only a kit's name:

=== "Node"

    ```bash
    copit init
    copit registry add chanx-kit-js github:chanx-dev/chanx-kit@v0.5.0 --to src/ws-kits
    copit registry add chanx-kit-ui github:chanx-dev/chanx-kit@v0.5.0 --to web/src/chanx-kit
    ```

=== "Python"

    ```bash
    copit init
    copit registry add chanx-kit github:chanx-dev/chanx-kit@v0.5.0 --to app/ws_kits
    copit registry add chanx-kit-ui github:chanx-dev/chanx-kit@v0.5.0 --to web/src/chanx-kit
    ```

=== "Go"

    ```bash
    copit init
    copit registry add chanx-kit-go github:chanx-dev/chanx-kit@v0.5.0 --to internal/wskits
    copit registry add chanx-kit-ui github:chanx-dev/chanx-kit@v0.5.0 --to web/src/chanx-kit
    ```

`--to` is where kits land; leave it out to take the suggested folder. copit 0.10 or newer
finds each registry by its name. Go kits need it: it runs `go get` for them and points
their imports at your module.

```toml
# copit.toml, written for you
[registries.chanx-kit-js]
source = "github:chanx-dev/chanx-kit@v0.5.0"
index = "js/copit-registry.json"
target = "src/ws-kits"
```

## 2. Add a kit

Server and browser halves in one command:

=== "Node"

    ```bash
    copit add @chanx-kit-js/notification @chanx-kit-ui/notification
    ```

=== "Python"

    ```bash
    copit add @chanx-kit/notification @chanx-kit-ui/notification
    ```

=== "Go"

    ```bash
    copit add @chanx-kit-go/notification @chanx-kit-ui/notification
    ```

The source lands in your project, and any package the kit needs is installed.
`--with tests` brings the kit's tests too; `--dry-run` shows the plan first.

## 3. Mount it

A kit is a **topic**: something a client subscribes to, with its own handlers and
authorization. List it on a consumer, the WebSocket route your clients connect to:

=== "Node"

    ```ts
    import { consumer } from '@chanx-js/server';

    import { BroadcastNotificationTopic, UserNotificationTopic } from './ws-kits/notification';

    export const app = consumer('/ws/notifications', {
      name: 'notifications',
      authenticate: async (request) => (await userFrom(request)) ?? false,
      topics: [UserNotificationTopic, BroadcastNotificationTopic],
    });
    ```

=== "Python"

    ```python
    from chanx.fast_channels.websocket import AsyncJsonWebsocketConsumer

    from .ws_kits.notification import (
        BroadcastNotificationTopic,
        NotificationMessage,
        UserNotificationTopic,
    )


    class AppConsumer(AsyncJsonWebsocketConsumer[NotificationMessage]):
        channel_layer_alias = "default"
        topics = [UserNotificationTopic, BroadcastNotificationTopic]
    ```

=== "Go"

    ```go
    type AppConsumer struct{}

    func (AppConsumer) Authenticate(_ *chanx.Context, r *chanx.Request) (any, error) {
    	return userFrom(r)
    }

    func (AppConsumer) Topics() []chanx.TopicHandler {
    	return []chanx.TopicHandler{
    		notification.UserNotificationTopic{},
    		notification.BroadcastNotificationTopic{},
    	}
    }
    ```

List as many kits as you need on one consumer; each keeps its own handlers and
authorization, and either works without the other.

## 4. Send from anywhere

A worker, a request handler or a signal reaches connected clients without touching the
consumer:

=== "Node"

    ```ts
    await notifyUser(chanx, user.id, { title: 'Build finished' });
    ```

=== "Python"

    ```python
    await UserNotificationTopic.notify_user(user.id, NotificationPayload(title="Build finished"))
    ```

=== "Go"

    ```go
    notification.NotifyUser(ctx, engine.Publisher(), user.ID,
    	notification.NotificationPayload{Title: "Build finished"})
    ```

!!! warning "Several processes need a shared layer"
    In memory, a broadcast stays in the process that sent it. Point every server and
    worker at the same Redis for anything multi-process.

## 5. Show it in the browser

The same on every server. Generate the client from your server's AsyncAPI schema, then
hand the channel and topics to the UI kit:

```tsx
import { NotificationFeed, useNotifications } from '@/chanx-kit/notification/react';
import '@/chanx-kit/notification/notification.css';
import { notifications } from '@/generated';

export function Inbox({ userId }: { userId: string }) {
  const mine = notifications.topics.userNotificationTopic.with({ user_id: userId });
  const { items, status, ackAll } = useNotifications(notifications, { topics: [mine] });
  return <NotificationFeed items={items} status={status} onAckAll={ackAll} />;
}
```

More in [UI kits](ui-kits.md).

## 6. Make it yours

The code is in your project: change it directly, or use the hooks each kit page lists.

=== "Node"

    ```ts
    const UserTopic = userNotificationTopic<User>({
      userId: (user) => user.accountId,
      onAcked: (ids) => store.markRead(ids),
    });
    ```

=== "Python"

    ```python
    class TenantNotificationTopic(UserNotificationTopic):
        async def on_notifications_acked(self, ids: list[str]) -> None:
            await Notification.objects.filter(id__in=ids).aupdate(read=True)
    ```

=== "Go"

    ```go
    notification.UserNotificationTopic{
    	Acks: notification.Acks{OnAcked: func(ctx *chanx.Context, ids []string) error {
    		return store.MarkRead(ctx, ids)
    	}},
    }
    ```

## Update kits

```bash
copit update-all                 # same pinned version
copit update-all --ref <newer tag>
```

Files you changed are kept; list them under `excludes` in `copit.toml` to skip them for
good.

## Next

Build a feature end to end: [a chat room with who's online](chat-room.md),
[an AI agent streaming to the page](agent-streaming.md) or [voice](voice.md). To see how
the pieces fit, read [how it works](how-it-works.md).
