// Run with npm run dev (API and WebSocket on :8000).
import { resolve } from 'node:path';

import fastifyStatic from '@fastify/static';
import websocket from '@fastify/websocket';
import { chanxFastify } from '@chanx-js/server/fastify';
import Fastify from 'fastify';

import { asyncapi, chanx, notify } from './app';

const app = Fastify({ logger: true });
await app.register(websocket);
await app.register(chanxFastify(chanx));
app.get('/asyncapi.json', () => chanx.asyncapi(asyncapi));
app.post<{ Body: { title: string; user?: string | null } }>('/api/notify', async (req) => {
  await notify(req.body.title, req.body.user);
  return { sent_to: req.body.user ?? 'everyone' };
});
// Serves the built UI (npm --prefix web run build).
await app.register(fastifyStatic, { root: resolve('web/dist') });

await app.listen({ port: 8000 });
