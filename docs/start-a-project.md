---
description: "Create a starter app on Express, Fastify, Hono, FastAPI, Django, net/http, chi, gin, Echo or Fiber, with React and a kit wired."
---

# Start a project on your framework

Every starter is the same app on a different framework: a server with the notification
kit and a React client typed from the server's schema. Create one with
[copit](getting-started.md#1-install-copit), choosing your framework below:

=== "Node"

    ```bash title="Express"
    copit create gh:chanx-dev/chanx-kit@v0.5.1/templates/express-react my-app
    ```

    ```bash title="Fastify"
    copit create gh:chanx-dev/chanx-kit@v0.5.1/templates/fastify-react my-app
    ```

    ```bash title="Hono"
    copit create gh:chanx-dev/chanx-kit@v0.5.1/templates/hono-react my-app
    ```

=== "Python"

    ```bash title="FastAPI"
    copit create gh:chanx-dev/chanx-kit@v0.5.1/templates/fastapi-react my-app
    ```

    ```bash title="Django"
    copit create gh:chanx-dev/chanx-kit@v0.5.1/templates/django-react my-app
    ```

=== "Go"

    ```bash title="net/http"
    copit create gh:chanx-dev/chanx-kit@v0.5.1/templates/go-nethttp-react my-app
    ```

    ```bash title="chi"
    copit create gh:chanx-dev/chanx-kit@v0.5.1/templates/go-chi-react my-app
    ```

    ```bash title="gin"
    copit create gh:chanx-dev/chanx-kit@v0.5.1/templates/go-gin-react my-app
    ```

    ```bash title="Echo"
    copit create gh:chanx-dev/chanx-kit@v0.5.1/templates/go-echo-react my-app
    ```

    ```bash title="Fiber"
    copit create gh:chanx-dev/chanx-kit@v0.5.1/templates/go-fiber-react my-app
    ```

Each one's README has its run commands, how to send from another process, and where
things live. Its `copit.toml` already points at this registry, so the next kit is one
command away:
=== "Node"

    ```bash
    copit add @chanx-kit-js/room-chat @chanx-kit-ui/chat
    ```

=== "Python"

    ```bash
    copit add @chanx-kit/room-chat @chanx-kit-ui/chat
    ```

=== "Go"

    ```bash
    copit add @chanx-kit-go/room-chat @chanx-kit-ui/chat
    ```

<small>Without copit, `npx tiged chanx-dev/chanx-kit/templates/<template> my-app` copies a
starter too; install copit before adding kits.</small>

## Next

Mount the kit you just added: [add kits to your app](add-to-your-app.md#3-mount-it),
from "Mount it". Or follow a guide end to end:
[a chat room with who's online](chat-room.md) or
[an AI agent streaming to the page](agent-streaming.md).
