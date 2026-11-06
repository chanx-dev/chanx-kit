#!/usr/bin/env node
// Drive a running sandbox (Python, Go or JS) over raw websockets: notifications, the
// chat room and presence. With a notify command, also a push from another process.
//
//   node scripts/sandbox_smoke.mjs http://127.0.0.1:8000 [notify command...]
//   node scripts/sandbox_smoke.mjs http://127.0.0.1:8000 uv run python -m sandbox.send_notification

import { execFileSync } from 'node:child_process';

const base = process.argv[2] ?? 'http://127.0.0.1:8000';
const notify = process.argv.slice(3);
let failed = false;
const check = (passed, what) => {
  console.log(`  ${passed ? 'ok  ' : 'FAIL'} ${what}`);
  failed ||= !passed;
};

function open(url) {
  const socket = new WebSocket(url);
  const frames = [];
  let waiters = [];
  socket.onmessage = (event) => {
    frames.push(JSON.parse(event.data));
    waiters = waiters.filter((waiter) => !waiter());
  };
  const until = (match, ms = 5000) =>
    new Promise((resolve) => {
      const timer = setTimeout(() => resolve(null), ms);
      const take = () => {
        const i = frames.findIndex(match);
        if (i < 0) return false;
        clearTimeout(timer);
        resolve(frames.splice(i, 1)[0]);
        return true;
      };
      if (!take()) waiters.push(take);
    });
  const ready = new Promise((resolve) => (socket.onopen = resolve));
  return { socket, until, ready, send: (frame) => socket.send(JSON.stringify(frame)) };
}

const ws = base.replace(/^http/, 'ws');
const about = await (await fetch(`${base}/api/sandbox`)).json();
console.log(`${base}: ${about.server} / ${about.framework}`);
const spec = await (await fetch(`${base}/asyncapi.json`)).json();
check(spec.channels?.notifications && spec.channels?.room, 'AsyncAPI lists notifications and room');
check((await (await fetch(`${base}/`)).text()).includes('id="root"'), 'serves the UI');

const chris = open(`${ws}/ws/notifications?as=chris`);
await chris.ready;
for (const topic of ['notification:user:chris', 'notification:all']) {
  chris.send({ action: 'subscribe', topic });
  check(await chris.until((f) => f.action === 'subscribed' && f.topic === topic), `subscribed to ${topic}`);
}
chris.send({ action: 'subscribe', topic: 'notification:user:bob' });
const refused = await chris.until((f) => f.action === 'error');
// Older Python chanx sends {reason}, RFC 9457 servers {type}.
check(
  refused?.payload?.type?.endsWith('/unauthorized') || refused?.payload?.reason === 'unauthorized',
  "another user's topic is refused",
);

if (notify.length) {
  const [command, ...args] = notify;
  execFileSync(command, [...args, '--user', 'chris', 'Just for you'], { stdio: 'pipe' });
  const mine = await chris.until((f) => f.action === 'notification' && f.topic === 'notification:user:chris', 8000);
  check(mine?.payload.title === 'Just for you' && mine.payload.id && mine.payload.created_at, 'notify --user reaches the user, defaults filled');
  execFileSync(command, [...args, 'Everyone'], { stdio: 'pipe' });
  check(await chris.until((f) => f.action === 'notification' && f.topic === 'notification:all', 8000), 'notify reaches everyone');
  chris.send({ action: 'notification_ack', topic: 'notification:user:chris', ref: 'ack', payload: { ids: [mine?.payload.id ?? 'x'] } });
  check((await chris.until((f) => f.action === 'notification_acked'))?.ref === 'ack', 'ack is confirmed');
}
chris.socket.close();

const room = `r${Date.now()}`;
const ann = open(`${ws}/ws/rooms/${room}?as=ann`);
const bob = open(`${ws}/ws/rooms/${room}?as=bob`);
await Promise.all([ann.ready, bob.ready]);
for (const client of [ann, bob]) {
  client.send({ action: 'subscribe', topic: `chat:${room}` });
  check(await client.until((f) => f.action === 'chat_backlog'), 'chat backlog on subscribe');
  client.send({ action: 'subscribe', topic: `presence:${room}` });
  await client.until((f) => f.action === 'presence_state');
}
const joined = await ann.until((f) => f.action === 'presence_join' && f.payload.member.id === 'bob');
check(joined?.payload.member.name === 'Bob', 'a join is announced with the display name');
ann.send({ action: 'chat_send', topic: `chat:${room}`, payload: { body: 'hello' } });
const message = await bob.until((f) => f.action === 'chat_message');
check(message?.payload.body === 'hello' && message.payload.author.id === 'ann' && message.payload.sent_at, 'a message reaches the room with its author');
ann.send({ action: 'chat_backlog_request', topic: `chat:${room}`, ref: 'b', payload: { limit: 5 } });
check((await ann.until((f) => f.action === 'chat_backlog' && f.ref === 'b'))?.payload.entries.length === 1, 'a history request replays it');
bob.socket.close();
check((await ann.until((f) => f.action === 'presence_leave'))?.payload.member.id === 'bob', 'a disconnect announces the leave');
ann.socket.close();
process.exit(failed ? 1 : 0);
