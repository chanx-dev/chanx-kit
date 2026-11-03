// Run with npm run dev (API and WebSocket on :8000).
import { createServer } from 'node:http';

import { asyncapiMiddleware, attachChanx } from '@chanx-js/server/node';
import express from 'express';

import { asyncapi, chanx, notify } from './app';

const app = express();
app.use(express.json());
app.use(asyncapiMiddleware(chanx, asyncapi));
app.post('/api/notify', async (req, res) => {
  await notify(req.body.title, req.body.user);
  res.json({ sent_to: req.body.user ?? 'everyone' });
});
// Serves the built UI (npm --prefix web run build).
app.use(express.static('web/dist'));

const server = createServer(app);
attachChanx(server, chanx);
server.listen(8000);
