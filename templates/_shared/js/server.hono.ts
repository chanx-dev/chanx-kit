// Run with npm run dev (API and WebSocket on :8000).
import { serve, upgradeWebSocket } from '@hono/node-server';
import { serveStatic } from '@hono/node-server/serve-static';
import { asyncapiHandler } from '@chanx-js/server';
import { chanxHono } from '@chanx-js/server/hono';
import { Hono } from 'hono';
import { WebSocketServer } from 'ws';

import { asyncapi, chanx, notify } from './app';

const app = new Hono();
const docs = asyncapiHandler(chanx, asyncapi);
app.get('/asyncapi*', async (c, next) => (await docs(c.req.raw)) ?? next());
app.get('/ws/*', chanxHono(chanx, upgradeWebSocket));
app.post('/api/notify', async (c) => {
  const { title, user } = await c.req.json<{ title: string; user?: string | null }>();
  await notify(title, user);
  return c.json({ sent_to: user ?? 'everyone' });
});
// Serves the built UI (npm --prefix web run build).
app.use('/*', serveStatic({ root: 'web/dist' }));

serve({
  fetch: app.fetch,
  port: 8000,
  websocket: { server: new WebSocketServer({ noServer: true }) },
});
