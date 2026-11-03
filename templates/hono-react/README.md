# chanx app

Hono and React on [chanx-js](https://github.com/chanx-dev/chanx-js), with the
[chanx-kit](https://github.com/chanx-dev/chanx-kit) `notification` kit wired end to
end. The client is typed from the server's AsyncAPI schema.

Needs Node 22+.

## Run

```bash
npm install
npm --prefix web install

npm run dev                 # API + WebSocket on :8000
npm --prefix web run dev    # UI on http://localhost:5173
```

Open two tabs and send a notification: both get it. "Only to" reaches just that user.
API docs: http://localhost:8000/asyncapi.

Single port: `npm --prefix web run build`, then run `npm start` alone.

## From another process

```bash
docker compose up -d
REDIS_URL=redis://localhost:6399/0 npm run dev
REDIS_URL=redis://localhost:6399/0 npm run notify -- "Build finished"
```

## Layout

| Path | |
| --- | --- |
| `src/app.ts` | Consumer and the kit topics it lists |
| `src/server.ts` | Routes on Hono, including `POST /api/notify` |
| `src/ws-kits/` | Kits copied by copit, yours to edit |
| `web/src/generated/` | Typed client, from `npm --prefix web run gen` |
| `web/src/chanx-kit/` | UI kits copied by copit: a core, React hooks and components, CSS |
| `copit.toml` | Kit sources, for `copit update` |

## Add a kit

Both registries are set up in `copit.toml`, so one command adds the server and browser
halves:

```bash
uvx copit add @chanx-kit-js/room-chat @chanx-kit-ui/chat
```

1. List its topics on a consumer in `src/app.ts`.
2. `npm --prefix web run gen`.
3. Pass the channel and topic from `@/generated` to the UI kit's hook, as `App.tsx` does
   with `useNotifications`.

Kits by feature, with the command for each:
https://kit.chanx.dev/kits/#by-feature

## Auth

Users are identified by `?as=<name>`: `authenticate` in `src/app.ts` returns the user
the kits read. Replace it with your auth.
