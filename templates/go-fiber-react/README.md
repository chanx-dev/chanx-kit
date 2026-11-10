# chanx app

Fiber on [chanx-go](https://github.com/chanx-dev/chanx-go) and React on
[chanx-js](https://github.com/chanx-dev/chanx-js), with the
[chanx-kit](https://github.com/chanx-dev/chanx-kit) `notification` kit wired end to
end. The client is typed from the server's AsyncAPI schema.

Needs Go 1.25+ and Node 20.19+.

## Run

```bash
npm --prefix web install

go run .                    # API + WebSocket on :8000
npm --prefix web run dev    # UI on http://localhost:5173
```

Open two tabs and send a notification: both get it. "Only to" reaches just that user.
API docs: http://localhost:8000/asyncapi.

Single port: `npm --prefix web run build`, then run `go run .` alone.

## From another process

```bash
docker compose up -d
REDIS_URL=redis://localhost:6399/0 go run .
REDIS_URL=redis://localhost:6399/0 go run ./cmd/notify "Build finished"
```

## Layout

| Path | |
| --- | --- |
| `internal/app/app.go` | Consumer and the kit topics it lists |
| `main.go` | Routes on Fiber, including `POST /api/notify` |
| `internal/wskits/` | Kits copied by copit, yours to edit |
| `web/src/generated/` | Typed client, from `npm --prefix web run gen` while the server runs (`gen:offline` without it) |
| `web/src/chanx-kit/` | UI kits copied by copit: a core, React hooks and components, CSS |
| `copit.toml` | Kit sources, for `copit update` |

## Add a kit

Both registries are set up in `copit.toml`, so one [copit](https://kit.chanx.dev/getting-started/#1-install-copit)
command adds the server and browser halves:

```bash
copit add @chanx-kit-go/room-chat @chanx-kit-ui/chat
```

1. List its topics on a consumer in `internal/app/app.go`.
2. `npm --prefix web run gen`.
3. Pass the channel and topic from `@/generated` to the UI kit's hook, as `App.tsx` does
   with `useNotifications`.

Kits by feature, with the command for each:
https://kit.chanx.dev/kits/#by-feature

## Auth

Users are identified by `?as=<name>`: `Authenticate` in `internal/app/app.go` returns
the user the kits read. Replace it with your auth.
