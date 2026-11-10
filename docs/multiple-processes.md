---
description: "Guide: reach browsers from workers and other replicas through Redis, across server languages."
---

# Send across processes with Redis

A notification from a background worker, a chat message from a cron job, the same app
on four replicas: each needs every process on one channel layer. In memory, a broadcast
stays in the process that sent it. With Redis it reaches every server and every worker.

## 1. Run Redis

Every starter has a `docker-compose.yml` for this:

```bash
docker compose up -d       # Redis on localhost:6399
```

## 2. Point the server at it

The starters already switch to Redis when `REDIS_URL` is set:

=== "Node"

    ```bash
    REDIS_URL=redis://localhost:6399/0 npm run dev
    ```

    The switch, in `src/app.ts`:

    ```ts
    const layer: ChannelLayer = process.env.REDIS_URL
      ? new RedisPubSubChannelLayer({ hosts: [process.env.REDIS_URL], prefix: 'chanx-app' })
      : new InMemoryChannelLayer();
    ```

=== "Python"

    ```bash
    REDIS_URL=redis://localhost:6399/0 uv run uvicorn app.main:app --reload
    ```

    The switch, in `app/layers.py`:

    ```python
    register_channel_layer(
        ALIAS, RedisPubSubChannelLayer(hosts=[redis_url], prefix="chanx-app")
    )
    ```

    On Django, set `CHANNEL_LAYERS` to a Redis layer the same way.

=== "Go"

    ```bash
    REDIS_URL=redis://localhost:6399/0 go run .
    ```

    The switch, in `internal/app/app.go`:

    ```go
    l, err := redis.NewPubSub(redis.PubSubOptions{Hosts: []string{url}, Prefix: "chanx-app"})
    ```

## 3. Send from another process

Each starter has a small sender. Run it with the same `REDIS_URL` while the browser is
open:

=== "Node"

    ```bash
    REDIS_URL=redis://localhost:6399/0 npm run notify -- --user chris "Build finished"
    ```

=== "Python"

    ```bash
    REDIS_URL=redis://localhost:6399/0 uv run python -m app.notify --user chris "Build finished"
    ```

=== "Go"

    ```bash
    REDIS_URL=redis://localhost:6399/0 go run ./cmd/notify --user chris "Build finished"
    ```

Chris's tab shows it, although the sender never opened a connection. Your own workers
do the same: set up the same layer, then call the kit's send function, such as
`notifyUser`, `post_to_room` or `PostToRoom`.

## Mix languages

Servers in different languages share a layer when they use the same Redis and the same
`prefix`. A Go worker can notify browsers connected to a Python server; each kit speaks
the same messages on every server.

## State that must be shared too

The layer carries messages. Kits that keep state need a shared store as well:

- **Presence:** add `redis-presence-store`, or each process sees only its own members.
- **Chat history:** give room-chat a database-backed store.
- **Agent runs:** the replay buffer, the one-run-per-thread claim and cancelling stay
  with the process running the run. Back the claim with Redis before scaling out; the
  [ag-ui](kits/ag-ui.md) page shows how.
