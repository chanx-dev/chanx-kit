import { createServer } from 'node:http';

import fastifyStatic from '@fastify/static';
import websocket from '@fastify/websocket';
import { serve as serveHono, upgradeWebSocket } from '@hono/node-server';
import { serveStatic } from '@hono/node-server/serve-static';
import type { Chanx } from '@chanx-js/server';
import { asyncapiHandler } from '@chanx-js/server';
import { chanxFastify } from '@chanx-js/server/fastify';
import { chanxHono } from '@chanx-js/server/hono';
import { asyncapiMiddleware, attachChanx } from '@chanx-js/server/node';
import express from 'express';
import Fastify from 'fastify';
import { Hono } from 'hono';
import Koa from 'koa';
import sirv from 'sirv';
import { WebSocketServer } from 'ws';

import { asyncapi } from './app';

export interface Serve {
  chanx: Chanx;
  port: number;
  ui: string;
  about: { server: 'js'; framework: string };
}

// The same consumers on each framework; only the mounting differs.
export const frameworks = {
  node: async ({ chanx, port, ui, about }: Serve) => {
    const docs = asyncapiMiddleware(chanx, asyncapi);
    const files = sirv(ui, { single: true });
    const server = createServer((req, res) => {
      if (req.url === '/api/sandbox') return void res.end(JSON.stringify(about));
      docs(req, res, () => files(req, res));
    });
    attachChanx(server, chanx);
    server.listen(port);
  },
  express: async ({ chanx, port, ui, about }: Serve) => {
    const app = express();
    app.use(asyncapiMiddleware(chanx, asyncapi));
    app.get('/api/sandbox', (_req, res) => void res.json(about));
    app.use(sirv(ui, { single: true }));
    const server = createServer(app);
    attachChanx(server, chanx);
    server.listen(port);
  },
  koa: async ({ chanx, port, ui, about }: Serve) => {
    const app = new Koa();
    const docs = asyncapiMiddleware(chanx, asyncapi);
    const files = sirv(ui, { single: true });
    app.use(async (ctx, next) => {
      if (ctx.path === '/api/sandbox') ctx.body = about;
      else await next();
    });
    app.use((ctx) => {
      ctx.respond = false;
      docs(ctx.req, ctx.res, () => files(ctx.req, ctx.res));
    });
    const server = createServer(app.callback());
    attachChanx(server, chanx);
    server.listen(port);
  },
  fastify: async ({ chanx, port, ui, about }: Serve) => {
    const app = Fastify();
    await app.register(websocket);
    await app.register(chanxFastify(chanx));
    await app.register(fastifyStatic, { root: ui });
    app.get('/asyncapi.json', () => chanx.asyncapi(asyncapi));
    app.get('/api/sandbox', () => about);
    await app.listen({ port });
  },
  hono: async ({ chanx, port, ui, about }: Serve) => {
    const app = new Hono();
    const docs = asyncapiHandler(chanx, asyncapi);
    app.get('/asyncapi*', async (c, next) => (await docs(c.req.raw)) ?? next());
    app.get('/api/sandbox', (c) => c.json(about));
    app.get('/ws/*', chanxHono(chanx, upgradeWebSocket));
    app.use('/*', serveStatic({ root: ui }));
    serveHono({ fetch: app.fetch, port, websocket: { server: new WebSocketServer({ noServer: true }) } });
  },
};
