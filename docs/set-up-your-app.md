---
description: "Add chanx to an existing app: the server for each framework, then the typed React client."
---

# Set up an existing app

Kits run on chanx: typed WebSocket consumers on your server, and a typed client in your
React app. Set both up once; then every kit is one `copit add`. Starting fresh? The
[starters](start-a-project.md) come with all of this done.

## Server

Each framework needs three things: the package, a **consumer** (the WebSocket route kits
are listed on), and the **AsyncAPI document** the browser's types are generated from.

### 1. Install and attach

=== "Node"

    ```bash
    npm install @chanx-js/server zod
    ```

    One file holds the consumers, whatever your framework. Kits go in `topics`:

    ```ts title="src/chanx.ts"
    import { consumer, createChanx } from '@chanx-js/server';

    export const app = consumer('/ws/app', { name: 'app', topics: [] });
    export const chanx = createChanx({ consumers: [app] });
    export const asyncapi = { title: 'My app', version: '1.0.0' };
    ```

    Then attach it to your framework:

    === "Express"

        ```bash
        npm install ws
        ```

        ```ts title="src/server.ts"
        import { createServer } from 'node:http';

        import { asyncapiMiddleware, attachChanx } from '@chanx-js/server/node';
        import express from 'express';

        import { asyncapi, chanx } from './chanx';

        const app = express();
        app.use(asyncapiMiddleware(chanx, asyncapi)); // /asyncapi and /asyncapi.json

        const server = createServer(app);
        attachChanx(server, chanx);
        server.listen(8000);
        ```

    === "Fastify"

        ```bash
        npm install @fastify/websocket
        ```

        ```ts title="src/server.ts"
        import websocket from '@fastify/websocket';
        import { chanxFastify } from '@chanx-js/server/fastify';
        import Fastify from 'fastify';

        import { asyncapi, chanx } from './chanx';

        const app = Fastify();
        await app.register(websocket);
        await app.register(chanxFastify(chanx));
        app.get('/asyncapi.json', () => chanx.asyncapi(asyncapi));

        await app.listen({ port: 8000 });
        ```

    === "Hono"

        ```bash
        npm install @hono/node-server ws
        ```

        ```ts title="src/server.ts"
        import { serve, upgradeWebSocket } from '@hono/node-server';
        import { asyncapiHandler } from '@chanx-js/server';
        import { chanxHono } from '@chanx-js/server/hono';
        import { Hono } from 'hono';
        import { WebSocketServer } from 'ws';

        import { asyncapi, chanx } from './chanx';

        const app = new Hono();
        const docs = asyncapiHandler(chanx, asyncapi);
        app.get('/asyncapi*', async (c, next) => (await docs(c.req.raw)) ?? next());
        app.get('/ws/*', chanxHono(chanx, upgradeWebSocket));

        serve({
          fetch: app.fetch,
          port: 8000,
          websocket: { server: new WebSocketServer({ noServer: true }) },
        });
        ```

=== "Python"

    === "FastAPI"

        ```bash
        pip install "chanx[fast_channels]" fastapi "uvicorn[standard]"
        ```

        ```python title="app/consumers.py"
        from chanx.core.decorators import channel
        from chanx.fast_channels.websocket import AsyncJsonWebsocketConsumer


        @channel(name="app")
        class AppConsumer(AsyncJsonWebsocketConsumer):
            channel_layer_alias = "default"
            topics = []  # kits go here
        ```

        ```python title="app/main.py"
        from chanx.fast_channels import asyncapi_spec_json
        from chanx.fast_channels.type_defs import AsyncAPIConfig
        from fast_channels.layers import InMemoryChannelLayer, register_channel_layer
        from fastapi import FastAPI, Request
        from fastapi.responses import JSONResponse

        from app.consumers import AppConsumer

        register_channel_layer("default", InMemoryChannelLayer())

        app = FastAPI()
        ws = FastAPI()
        ws.router.add_websocket_route("/app", AppConsumer.as_asgi())
        app.mount("/ws", ws)


        @app.get("/asyncapi.json")
        async def asyncapi_json(request: Request) -> JSONResponse:
            config = AsyncAPIConfig(version="1.0.0")
            return await asyncapi_spec_json(request=request, app=app, config=config)
        ```

        Run it with `uvicorn app.main:app --port 8000`.

    === "Django"

        ```bash
        pip install "chanx[channels]" "channels[daphne]"
        ```

        ```python title="config/settings.py"
        INSTALLED_APPS = [
            "daphne",  # first, so runserver serves WebSockets
            # ... Django's own apps ...
            "channels",
            "chanx.channels",
            "app",
        ]
        ASGI_APPLICATION = "config.asgi.application"
        CHANNEL_LAYERS = {"default": {"BACKEND": "channels.layers.InMemoryChannelLayer"}}
        ```

        ```python title="app/consumers.py"
        from chanx.channels.websocket import AsyncJsonWebsocketConsumer
        from chanx.core.decorators import channel


        @channel(name="app")
        class AppConsumer(AsyncJsonWebsocketConsumer):
            topics = []  # kits go here
        ```

        ```python title="config/asgi.py"
        import os

        from django.core.asgi import get_asgi_application

        os.environ.setdefault("DJANGO_SETTINGS_MODULE", "config.settings")
        django_asgi_app = get_asgi_application()

        from channels.routing import ProtocolTypeRouter, URLRouter
        from chanx.channels.routing import path

        from app.consumers import AppConsumer

        application = ProtocolTypeRouter({
            "http": django_asgi_app,
            "websocket": URLRouter([path("ws/app", AppConsumer.as_asgi())]),
        })
        ```

        Add `path("asyncapi/", include("chanx.channels.urls"))` to `config/urls.py` for
        the docs page, and run it with `python manage.py runserver 8000`.

=== "Go"

    ```bash
    go get go.chanx.dev/v2
    ```

    One engine holds the consumers, whatever your router. Kits go in `Topics`:

    ```go title="internal/app/app.go"
    package app

    import (
    	chanx "go.chanx.dev/v2"
    	"go.chanx.dev/v2/asyncapi"
    )

    var Info = asyncapi.Info{Title: "My app", Version: "1.0.0"}

    type AppConsumer struct{}

    func (AppConsumer) Topics() []chanx.TopicHandler {
    	return nil // kits go here
    }

    func New() *chanx.Engine {
    	engine := chanx.New(chanx.Config{})
    	engine.Handle("/ws/app", AppConsumer{}, chanx.Name("app"))
    	return engine
    }
    ```

    Then mount it on your router:

    === "net/http"

        ```go title="main.go"
        engine := app.New()
        mux := http.NewServeMux()
        chanxhttp.Mount(mux, engine) // chanxhttp "go.chanx.dev/v2/nethttp"
        chanxhttp.MountDocs(mux, engine, "/asyncapi", app.Info)
        log.Fatal(http.ListenAndServe(":8000", mux))
        ```

    === "chi"

        ```go title="main.go"
        engine := app.New()
        r := chi.NewRouter()
        chanxhttp.Mount(r, engine, chanxhttp.NoMethod()) // chanxhttp "go.chanx.dev/v2/nethttp"
        r.Get("/asyncapi.json", asyncapi.JSONHandler(engine, app.Info).ServeHTTP)
        log.Fatal(http.ListenAndServe(":8000", r))
        ```

    === "gin"

        ```bash
        go get go.chanx.dev/adapters/gin/v2
        ```

        ```go title="main.go"
        engine := app.New()
        r := gin.Default()
        chanxgin.Mount(r, engine)
        chanxgin.MountDocs(r, engine, "/asyncapi", app.Info)
        log.Fatal(r.Run(":8000"))
        ```

    === "Echo"

        ```bash
        go get go.chanx.dev/adapters/echo/v2
        ```

        ```go title="main.go"
        engine := app.New()
        e := echo.New()
        chanxecho.Mount(e, engine)
        chanxecho.MountDocs(e, engine, "/asyncapi", app.Info)
        log.Fatal(e.Start(":8000"))
        ```

    === "Fiber"

        ```bash
        go get go.chanx.dev/adapters/fiber/v2
        ```

        ```go title="main.go"
        engine := app.New()
        server := fiber.New()
        chanxfiber.Mount(server, engine)
        chanxfiber.MountDocs(server, engine, "/asyncapi", app.Info)
        log.Fatal(server.Listen(":8000"))
        ```

### 2. Check it

With the server running, the AsyncAPI document lists your consumer:

=== "Node"

    ```bash
    curl http://localhost:8000/asyncapi.json
    ```

=== "Python"

    ```bash
    curl http://localhost:8000/asyncapi.json                        # FastAPI
    python manage.py generate_asyncapi_schema --format json         # Django
    ```

=== "Go"

    ```bash
    curl http://localhost:8000/asyncapi.json
    ```

## Browser

Any React app works; these steps assume Vite, with the app in `web/`.

### 3. Install the client

```bash
npm --prefix web install @chanx-js/client
npm --prefix web install -D @chanx-js/codegen
```

### 4. Generate the typed client

The generator reads the AsyncAPI document straight from your running server. Add it as
a `gen` script in `web/package.json` and run it whenever a consumer or kit changes:

```json title="web/package.json"
"gen": "chanx-codegen http://localhost:8000/asyncapi.json -o src/generated"
```

On Django the document is at `http://localhost:8000/asyncapi/schema/`.

??? note "Without a running server, such as in CI"

    Write the document to a file from your server's code, then point the generator at
    the file: `chanx-codegen asyncapi.json -o src/generated`. The starters have this as
    `npm run gen:offline`; on Django the document comes from
    `python manage.py generate_asyncapi_schema --format json`.

`src/generated` now exports one descriptor per consumer, such as `app`, with its topics.

### 5. Reach the server

The client resolves `/ws/...` against the page's own address. In development the page
is on Vite's port, so proxy WebSockets to the server:

```ts title="web/vite.config.ts"
export default defineConfig({
  plugins: [react()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: { proxy: { '/ws': { target: 'ws://localhost:8000', ws: true } } },
});
```

If the API lives on another origin in production, point the client at it once, before
rendering:

```ts title="web/src/main.tsx"
import { setDefaultClient } from '@chanx-js/client/react';

setDefaultClient({ baseUrl: 'wss://api.example.com' });
```

### 6. Add the `@` alias

Kit examples import from `@/chanx-kit/...` and `@/generated`. The `resolve.alias` above
covers Vite; tell TypeScript too:

```json title="web/tsconfig.app.json"
"compilerOptions": { "paths": { "@/*": ["./src/*"] } }
```

That is `tsconfig.app.json` in Vite's template, or `tsconfig.json` if you have only one.

Or use relative imports; the kits' own files do.

## Next

Server and browser are ready: [add your first kit](add-to-your-app.md). In production,
point every process at one Redis; see [sending across processes](multiple-processes.md).
