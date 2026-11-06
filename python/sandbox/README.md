# sandbox

A FastAPI app with every kit mounted, plus the UI that drives them. It is not published
or copied anywhere: it exists so the kits can be run for real, and it does two jobs
beyond the demo.

1. **The reference for composing kits.** `consumers.py` is what a real project's
   consumers should look like, with independent kits listed side by side.
2. **The source of the wire contract.** The TypeScript client is generated from this
   app's AsyncAPI schema, so a change to any kit's messages shows up as a TypeScript
   diff rather than a runtime surprise.

## Run it

```bash
uv sync
npm --prefix ui/sandbox install && npm --prefix ui/sandbox run build
uv run python -m sandbox
```

Voice demos use Deepgram and ElevenLabs when `DEEPGRAM_API_KEY` / `ELEVENLABS_API_KEY`
are set, in the environment or a git-ignored `.env` at the repo root, and the fake
provider otherwise. The startup banner says which.

Open **http://localhost:8000** for the demos, one page each in a sidebar
(`src/demos.tsx`), and **http://localhost:8000/asyncapi** for the generated WebSocket
API docs. Each demo has its own link, such as `/#/voice-agent`, and only the open one
holds a socket. The startup banner tells you which channel layer is active and whether
the UI has been built.

For UI work, run Vite separately so you get hot reload. It proxies `/ws` to the server:

```bash
uv run python -m sandbox          # terminal 1
npm --prefix ui/sandbox run dev   # terminal 2, http://localhost:5173
```

## Routes

| Path | Consumer | Kits |
|---|---|---|
| `/ws/notifications` | `NotificationConsumer` | notification |
| `/ws/rooms/{room}` | `RoomConsumer` | room-chat, presence |
| `/ws/agent` | `AgentConsumer` | ag-ui |
| `/ws/voice` | `VoiceConsumer` | audio-stream-in, audio-stream-out |
| `/ws/assistant` | `AssistantConsumer` | voice-agent |

Open the chat room in a second tab to watch history replay and the roster update live.

## Across processes

The default channel layer is in-memory, so it cannot leave the process. To see a worker
reach the browser, give both sides the same Redis. Every sandbox command reads `.env`,
so set `REDIS_URL` there once:

```bash
docker compose up -d
cp .env.example .env        # or add its REDIS_URL line to your .env
uv run python -m sandbox    # the banner says "channel layer  redis (...)"
uv run python -m sandbox.send_notification "Build finished"
```

That reaches every open tab, on `notification:all`. To address one audience instead, and
watch the server refuse a client that asks for someone else's:

```bash
uv run python -m sandbox.send_notification --user chris "Just for you"
```

`send_notification.py` is the shape of a Django signal or a Celery task: no WebSocket and
no consumer, just a topic. `worker.py` is the same idea for an agent run: start one in the
UI, then feed tool calls and streamed text into it from another process.

```bash
uv run python -m sandbox.worker <run-id>
```

Both publish through the same topic classes `consumers.py` mounts, and that matters:
group names are namespaced by class name, so a subclass of `UserNotificationTopic` would
address a different group. Publishing from the wrong one is silent, since a broadcast
with no subscribers is not an error. That is why identity comes from `DemoIdentity`, a
middleware putting `?as=<name>` on the scope as the user, rather than from subclasses.

## Go and JS servers

The same UI runs against the Go and JS kits, on the framework you pick. Build the UI
once, then start one server instead of the Python one:

```bash
go -C go/sandbox run . -framework gin          # nethttp, chi, gin, echo, fiber
npm --prefix js run sandbox -- --framework hono # node, express, koa, fastify, hono
```

They serve the demos whose kits they have, notifications, the chat room and AG-UI, and the
sidebar says which server and framework you are on. With the same `REDIS_URL`, a Python,
Go and JS server share one room: the kits use the same group names and the layers the
same prefix. Each has its own notify command:

```bash
go -C go/sandbox run . notify -user chris "Just for you"
npm --prefix js run sandbox:notify -- --user chris "Just for you"
```

## Files

| File | What it is |
|---|---|
| `consumers.py` | Every kit composed onto three consumers. Start here |
| `main.py` | The FastAPI app, WebSocket routes and AsyncAPI docs |
| `layers.py` | Redis when `REDIS_URL` is set, in-memory otherwise |
| `__main__.py` | `python -m sandbox`, with the startup banner |
| `send_notification.py`, `worker.py` | Publishing from outside the web process |
| `ui/sandbox/` | The React demo, shared by all three servers, with `src/generated/` written by the codegen and `src/chanx-kit/` installed from the UI registry by copit |
| `go/sandbox/`, `js/sandbox/` | The Go and JS servers for the same UI, one per framework |

## After changing a kit

Regenerate the client, or CI fails on the diff:

```bash
npm --prefix ui/sandbox run gen
```

A new kit also has to be mounted in `consumers.py`. A kit that is absent from this app is
absent from the AsyncAPI schema and therefore has no contract check at all, so
`tests/test_sandbox.py` asserts every kit's messages are reachable from here.

## Checking it end to end

With a sandbox running, these drive it for real; each prints one line per check:

```bash
node scripts/sandbox_smoke.mjs http://127.0.0.1:8000 uv run python -m sandbox.send_notification
node scripts/sandbox_ui_e2e.mjs http://127.0.0.1:8000     # a browser, through Playwright
node scripts/sandbox_cross.mjs    # Python :8000, Go :8001 and JS :8002 on one Redis
```

Drop the notify command to skip the cross-process push. `sandbox_ui_e2e.mjs` takes the
Playwright module from `PLAYWRIGHT`, as `sandbox_voice_e2e.mjs` does.
