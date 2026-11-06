#!/usr/bin/env node
// Python (:8000), Go (:8001) and JS (:8002) sandboxes on one Redis reach each other's clients.
//
//   node scripts/sandbox_cross.mjs

import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';

const repo = resolve(import.meta.dirname, '..');
const servers = { python: 8000, go: 8001, js: 8002 };
const senders = {
  python: ['uv', ['run', 'python', '-m', 'sandbox.send_notification'], repo],
  go: ['go', ['run', '.', 'notify'], `${repo}/go/sandbox`],
  js: ['npx', ['tsx', 'sandbox/notify.ts'], `${repo}/js`],
};
let failed = false;
const check = (passed, what) => {
  console.log(`  ${passed ? 'ok  ' : 'FAIL'} ${what}`);
  failed ||= !passed;
};

function open(server, path) {
  const socket = new WebSocket(`ws://127.0.0.1:${servers[server]}${path}`);
  const frames = [];
  let waiters = [];
  socket.onmessage = (event) => {
    frames.push(JSON.parse(event.data));
    waiters = waiters.filter((waiter) => !waiter());
  };
  const until = (match, ms = 8000) =>
    new Promise((done) => {
      const timer = setTimeout(() => done(null), ms);
      const take = () => {
        const i = frames.findIndex(match);
        if (i < 0) return false;
        clearTimeout(timer);
        done(frames.splice(i, 1)[0]);
        return true;
      };
      if (!take()) waiters.push(take);
    });
  const ready = new Promise((done) => (socket.onopen = done));
  return { socket, until, ready, send: (frame) => socket.send(JSON.stringify(frame)) };
}

const all = async (clients) => Promise.all(Object.values(clients).map((c) => c.ready));
const each = (make) => Object.fromEntries(Object.keys(servers).map((s) => [s, make(s)]));

// One user with a tab on every server.
const tabs = each((s) => open(s, '/ws/notifications?as=dana'));
await all(tabs);
for (const tab of Object.values(tabs)) {
  tab.send({ action: 'subscribe', topic: 'notification:user:dana' });
  await tab.until((f) => f.action === 'subscribed');
}
for (const [from, [command, args, cwd]] of Object.entries(senders)) {
  execFileSync(command, [...args, '--user', 'dana', `from ${from}`], { cwd, stdio: 'pipe' });
  for (const [to, tab] of Object.entries(tabs)) {
    check(await tab.until((f) => f.payload?.title === `from ${from}`), `${from} notify reaches a client on ${to}`);
  }
}

// One room, a person on each server.
const room = `x${Date.now()}`;
const people = { python: 'pia', go: 'gus', js: 'jo' };
const rooms = each((s) => open(s, `/ws/rooms/${room}?as=${people[s]}`));
await all(rooms);
for (const client of Object.values(rooms)) {
  client.send({ action: 'subscribe', topic: `chat:${room}` });
  await client.until((f) => f.action === 'chat_backlog');
  client.send({ action: 'subscribe', topic: `presence:${room}` });
  await client.until((f) => f.action === 'presence_state');
}
for (const [from, client] of Object.entries(rooms)) {
  client.send({ action: 'chat_send', topic: `chat:${room}`, payload: { body: `hi from ${from}` } });
  for (const [to, other] of Object.entries(rooms)) {
    const message = await other.until((f) => f.payload?.body === `hi from ${from}`);
    check(message?.payload.author.id === people[from], `chat from ${from} reaches ${to}`);
  }
}
check(await rooms.python.until((f) => f.action === 'presence_join' && f.payload.member.id === 'jo'), 'a JS join reaches Python');
rooms.go.socket.close();
check(await rooms.js.until((f) => f.action === 'presence_leave' && f.payload.member.id === 'gus'), 'a Go leave reaches JS');
process.exit(failed ? 1 : 0);
