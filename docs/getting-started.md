---
description: "Tutorial: a live notification inbox on Node, Python or Go, then a second kit added with one command."
---

# Build a live notification inbox

In about fifteen minutes you will have a web page that shows a notification the moment
your server sends it, in every open tab, and a live list of who is online, added with one
command.

Pick your server once; every step below follows it.

**You need** Node 22 or newer, plus your server's toolchain:

=== "Node"

    Nothing else.

=== "Python"

    [uv](https://docs.astral.sh/uv/getting-started/installation/).

=== "Go"

    Go 1.25 or newer.

## 1. Install copit

--8<-- "docs/.snippets/install-copit.md"

You use it twice here: to create the app now, and to add a second feature later.

## 2. Create the starter app

=== "Node"

    ```bash
    copit create gh:chanx-dev/chanx-kit@v0.5.0/templates/express-react inbox
    cd inbox
    ```

=== "Python"

    ```bash
    copit create gh:chanx-dev/chanx-kit@v0.5.0/templates/fastapi-react inbox
    cd inbox
    ```

=== "Go"

    ```bash
    copit create gh:chanx-dev/chanx-kit@v0.5.0/templates/go-nethttp-react inbox
    cd inbox
    ```

You now have a server, a React app in `web/`, and the notification kit already copied
into both.

## 3. Start the server

=== "Node"

    ```bash
    npm install
    npm --prefix web install
    npm run dev
    ```

=== "Python"

    ```bash
    uv sync
    npm --prefix web install
    uv run uvicorn app.main:app --reload
    ```

=== "Go"

    ```bash
    npm --prefix web install
    go run .
    ```

The server listens on port 8000. Leave it running.

## 4. Open the inbox

In a second terminal:

```bash
npm --prefix web run dev
```

Open <http://localhost:5173>. You see **chanx app** with an empty notification feed,
its status reading **open**: the page holds a live connection to your server.

## 5. Send from the page

Open the same address in a second browser tab. In either tab, type a title and press
**Send**.

The notification appears in **both** tabs at once.

## 6. Send from your terminal

Nothing on the sending side needs a WebSocket. Any process that can reach your server
can push:

```bash
curl -X POST http://localhost:8000/api/notify \
  -H 'Content-Type: application/json' -d '{"title": "Build finished"}'
```

"Build finished" appears in both tabs.

## 7. Send to one person

Each tab connects as the name in **Connect as**, `chris` by default. Change one tab to
`ann`, then:

```bash
curl -X POST http://localhost:8000/api/notify \
  -H 'Content-Type: application/json' -d '{"title": "Just for Ann", "user": "ann"}'
```

Only Ann's tab shows it. The server refused to let Chris's tab subscribe to Ann's
notifications in the first place.

## 8. Add a second kit

Every feature is one copit command. Add presence, the server half and the browser half
together. The starter already knows where kits come from (its `copit.toml`), so there is
nothing to set up first:

=== "Node"

    ```bash
    copit add @chanx-kit-js/presence @chanx-kit-ui/presence -y
    ```

=== "Python"

    ```bash
    copit add @chanx-kit/presence @chanx-kit-ui/presence -y
    ```

=== "Go"

    ```bash
    copit add @chanx-kit-go/presence @chanx-kit-ui/presence -y
    ```

Two new folders appear: the server kit next to `notification`, and
`web/src/chanx-kit/presence`.

## 9. Mount it

List the presence topic next to the notification ones:

=== "Node"

    In `src/app.ts`:

    ```ts
    import { PresenceTopic } from './ws-kits/presence';

    // in consumer('/ws/notifications', { ... })
    topics: [UserNotificationTopic, BroadcastNotificationTopic, PresenceTopic],
    ```

=== "Python"

    In `app/consumers.py`:

    ```python
    from app.ws_kits.presence import PresenceTopic

    # in NotificationConsumer
    topics: ClassVar[list[type[Topic[Any]]]] = [
        UserNotificationTopic,
        BroadcastNotificationTopic,
        PresenceTopic,
    ]
    ```

=== "Go"

    In `internal/app/app.go`, then restart `go run .`:

    ```go
    import "app/internal/wskits/presence"

    // in NotificationConsumer.Topics
    return []chanx.TopicHandler{
    	notification.UserNotificationTopic{},
    	notification.BroadcastNotificationTopic{},
    	presence.PresenceTopic{},
    }
    ```

Then regenerate the typed client from the server's schema:

```bash
npm --prefix web run gen
```

`web/src/generated/channels.ts` now has a `presenceTopic`.

## 10. Show who is here

In `web/src/App.tsx`, next to `useNotifications`:

```tsx
import { PresenceRoster, usePresence } from '@/chanx-kit/presence/react';
import '@/chanx-kit/presence/presence.css';

// inside App(), with the same options, so it shares the socket:
const { members } = usePresence(
  notifications,
  notifications.topics.presenceTopic.with({ scope: 'lobby' }),
  { queryParams: { as: who } },
);

// in the JSX, under the header:
<PresenceRoster members={members} self={who} />
```

Both tabs now list **chris** and **ann**. Close one, and the other drops it.

## What you built

A server that pushes to the right browser tabs from anywhere, a React inbox that
stays typed to the server's own schema, a live roster added with one command, and the
code for all of it in your project, yours to edit.

## Next

- **Make it your own project:** [start a project](start-a-project.md) on your framework,
  or [set up an existing app](set-up-your-app.md).
- **Build another feature:** [a chat room with who's online](chat-room.md) or
  [an AI agent streaming to the page](agent-streaming.md).
- **Understand it:** [how it works](how-it-works.md).
