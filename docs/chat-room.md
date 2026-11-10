---
description: "Guide: a chat room with history and a live roster, on Node, Python or Go with React."
---

# Add a chat room with who's online

A room where history replays on join, new messages arrive live, and a roster shows who
is there. It is two kits, room-chat and presence, on one route. Start from any
[starter](start-a-project.md) or [your own app](add-to-your-app.md).

## 1. Add the kits

=== "Node"

    ```bash
    copit add @chanx-kit-js/room-chat @chanx-kit-js/presence @chanx-kit-ui/chat
    ```

=== "Python"

    ```bash
    copit add @chanx-kit/room-chat @chanx-kit/presence @chanx-kit-ui/chat
    ```

=== "Go"

    ```bash
    copit add @chanx-kit-go/room-chat @chanx-kit-go/presence @chanx-kit-ui/chat
    ```

The chat UI kit brings the presence UI kit with it.

## 2. Mount a room route

The room is part of the path, so each room is its own connection scope. List both
topics on it:

=== "Node"

    In `src/app.ts`:

    ```ts
    import { PresenceTopic } from './ws-kits/presence';
    import { RoomChatTopic } from './ws-kits/room-chat';

    export const room = consumer('/ws/rooms/{room}', {
      name: 'room',
      authenticate,
      topics: [RoomChatTopic, PresenceTopic],
    });

    export const chanx = createChanx({ consumers: [notifications, room], layers: { default: layer } });
    ```

=== "Python"

    In `app/consumers.py`, then route it in `app/main.py`:

    ```python
    from app.ws_kits.presence import PresenceJoinMessage, PresenceLeaveMessage, PresenceTopic
    from app.ws_kits.room_chat import ChatMessage, RoomChatTopic


    @channel(name="room", description="Room chat with a live roster.")
    class RoomConsumer(
        AsyncJsonWebsocketConsumer[ChatMessage | PresenceJoinMessage | PresenceLeaveMessage]
    ):
        channel_layer_alias = ALIAS
        topics: ClassVar[list[type[Topic[Any]]]] = [RoomChatTopic, PresenceTopic]
    ```

    ```python
    ws.router.add_websocket_route("/rooms/{room}", RoomConsumer.as_asgi())
    ```

=== "Go"

    In `internal/app/app.go`:

    ```go
    type RoomConsumer struct{ NotificationConsumer }

    func (RoomConsumer) Topics() []chanx.TopicHandler {
    	return []chanx.TopicHandler{roomchat.RoomChatTopic{}, presence.PresenceTopic{}}
    }

    // next to the notifications route:
    engine.Handle("/ws/rooms/{room}", RoomConsumer{}, chanx.Name("room"))
    ```

    `RoomConsumer` embeds the starter's consumer to reuse its `Authenticate`.

Restart the server, then regenerate the typed client:

```bash
npm --prefix web run gen
```

## 3. Show the room

```tsx
import { ChatRoom, useChat } from '@/chanx-kit/chat/react';
import { usePresence } from '@/chanx-kit/presence/react';
import '@/chanx-kit/chat/chat.css';
import '@/chanx-kit/presence/presence.css';
import { room } from '@/generated';

export function Room({ name, me }: { name: string; me: string }) {
  // Same channel and options, so both hooks share one socket.
  const options = { params: { room: name }, queryParams: { as: me } };
  const { entries, status, send } = useChat(room, room.topics.roomChatTopic.with({ room: name }), options);
  const { members } = usePresence(room, room.topics.presenceTopic.with({ scope: name }), options);
  return <ChatRoom entries={entries} status={status} onSend={send} members={members} self={me} />;
}
```

Open it in two tabs as two names: messages and the roster update in both. `queryParams`
is how the starters fake sign-in; drop it once your `authenticate` reads a real session.

## 4. Post from the server

A bot, a deploy hook or a moderator posts without a connection:

=== "Node"

    ```ts
    await RoomChatTopic.postToRoom(chanx, 'general', 'Deploy finished', { id: 'ci' });
    ```

=== "Python"

    ```python
    await RoomChatTopic.post_to_room("general", "Deploy finished", ChatAuthor(id="ci"))
    ```

=== "Go"

    ```go
    roomchat.RoomChatTopic{}.PostToRoom(ctx, engine.Publisher(), "general", "Deploy finished",
    	roomchat.ChatAuthor{ID: "ci"})
    ```

## Before production

- **Keep history somewhere durable.** The default store is in memory and per process.
  Give the topic your own message store; on Django, `@chanx-kit/django-message-store`
  is one.
- **Share the roster across processes.** Add `redis-presence-store`, and run every
  process on [one Redis](multiple-processes.md).
- **Decide who may join.** Each topic has its own authorization hook; the
  [room-chat](kits/room-chat.md) and [presence](kits/presence.md) pages list them.
