---
hide:
  - navigation
  - toc
---

# ![chanx-kit](assets/brand/chanx-kit-wordmark.svg#only-light){ .hero-wordmark } ![chanx-kit](assets/brand/chanx-kit-wordmark-dark.svg#only-dark){ .hero-wordmark }

**Realtime features you copy into your app.** Notifications, chat, presence, AI agent
streaming and voice, for Node, Python and Go servers, with React UI that works with any
of them.
{ .hero-lead }

[Build one in 15 minutes](getting-started.md){ .md-button .md-button--primary }
[Browse the kits](kits/index.md){ .md-button }

## Compose features, not plumbing

Each kit is one feature. Add as many as you need, list them on the same WebSocket route,
and change any of them in place. Here, a chat room and its live roster:

=== "Node"

    ```bash
    copit add @chanx-kit-js/room-chat @chanx-kit-js/presence @chanx-kit-ui/chat
    ```

    ```ts
    // Tuned in place; swap its store the same way.
    const Chat = roomChatTopic({ backlogLimit: 100 });

    export const room = consumer('/ws/rooms/{room}', {
      name: 'room',
      authenticate,
      topics: [Chat, PresenceTopic],
    });

    // From a worker, a webhook, anywhere:
    await Chat.postToRoom(chanx, 'general', 'Deploy finished', { id: 'ci' });
    ```

=== "Python"

    ```bash
    copit add @chanx-kit/room-chat @chanx-kit/presence @chanx-kit-ui/chat
    ```

    ```python
    # Tuned in place; swap its store the same way.
    class ChatTopic(RoomChatTopic):
        backlog_limit = 100


    RoomMessage = ChatMessage | PresenceJoinMessage | PresenceLeaveMessage


    class RoomConsumer(AsyncJsonWebsocketConsumer[RoomMessage]):
        topics = [ChatTopic, PresenceTopic]


    # From a worker, a signal, anywhere:
    await ChatTopic.post_to_room("general", "Deploy finished", ChatAuthor(id="ci"))
    ```

=== "Go"

    ```bash
    copit add @chanx-kit-go/room-chat @chanx-kit-go/presence @chanx-kit-ui/chat
    ```

    ```go
    // Tuned in place; swap its Store the same way.
    var chat = roomchat.RoomChatTopic{BacklogLimit: 100}

    func (RoomConsumer) Topics() []chanx.TopicHandler {
    	return []chanx.TopicHandler{chat, presence.PresenceTopic{}}
    }

    // From a worker, a handler, anywhere:
    chat.PostToRoom(ctx, pub, "general", "Deploy finished", roomchat.ChatAuthor{ID: "ci"})
    ```

In the browser, the same for every server. Both hooks share one socket:

```tsx
const { entries, status, send } = useChat(room, room.topics.roomChatTopic.with({ room: 'general' }), options);
const { members } = usePresence(room, room.topics.presenceTopic.with({ scope: 'general' }), options);
return <ChatRoom entries={entries} status={status} onSend={send} members={members} self={me} />;
```

Each topic keeps its own handlers and authorization, so either kit works without the
other, and the next feature (notifications, an AI agent) is one more line in `topics`.

## What you can add

<div class="grid cards three compact" markdown>

-   **[Notifications](kits/notification.md)**

    Push to a user's open tabs from a worker or another service.

    <span class="servers">Node · Python · Go</span>

-   **[Chat room](chat-room.md)**

    History replayed on join, then every message live.

    <span class="servers">Node · Python · Go</span>

-   **[Presence](kits/presence.md)**

    Who is in a room, document or tenant, as it changes.

    <span class="servers">Node · Python · Go</span>

-   **[Agent streaming](agent-streaming.md)**

    Any agent's events to the browser over AG-UI; cancel, share a run.

    <span class="servers">Node · Python · Go</span>

-   **[Pydantic AI agent](kits/pydantic-ai-ag-ui.md)**

    A Pydantic AI agent over AG-UI, approvals included.

    <span class="servers">Python</span>

-   **[Voice](voice.md)**

    Live captions, text to speech, a voice agent you can interrupt.

    <span class="servers">Python</span>

-   **[Every kit :material-arrow-right:](kits/index.md#catalog)**

    Stores, providers and the rest, with the servers each runs on.

</div>

## Why chanx-kit

<div class="grid cards three" markdown>

-   :material-pipe-wrench: **Stop rebuilding the plumbing**

    ---

    Groups, per-subscriber authorization, state on connect, cleanup on disconnect,
    reaching a browser from a worker: written once, tested, on Express, Fastify, Hono,
    FastAPI, Django, net/http, gin, Echo and Fiber.

-   :material-file-code-outline: **Own the code**

    ---

    One command copies readable source into your project. Edit it, delete half of it.
    No framework owns your connections.

-   :material-handshake-outline: **Both ends always agree**

    ---

    The browser's types come from your server's schema, and every server language is
    checked against the same messages. Swap Node for Go and the UI does not notice.

</div>

How the pieces fit: [how it works](how-it-works.md). Using an AI coding assistant? Point it
at [`llms.txt`](llms.txt), or every page at once in [`llms-full.txt`](llms-full.txt).

<small>Built on [chanx](https://github.com/chanx-dev/chanx),
[chanx-go](https://go.chanx.dev/) and [chanx-js](https://js.chanx.dev/), one protocol
across three languages, and copied in with
[copit](https://github.com/huynguyengl99/copit). Pronounced "chan-X kit".</small>
