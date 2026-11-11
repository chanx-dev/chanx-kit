#!/usr/bin/env node
// Drive the sandbox's messaging demos in a real browser, against any sandbox server.
//
//   PLAYWRIGHT=/path/to/node_modules/playwright/index.mjs \
//     node scripts/sandbox_ui_e2e.mjs http://127.0.0.1:8000 [notify command...]

import { execFileSync } from 'node:child_process';

const base = process.argv[2] ?? 'http://127.0.0.1:8000';
const notify = process.argv.slice(3);
const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright');

let failed = false;
const check = (passed, what) => {
  console.log(`  ${passed ? 'ok  ' : 'FAIL'} ${what}`);
  failed ||= !passed;
};
const shows = (locator, state = 'visible') =>
  locator.waitFor({ state, timeout: 8000 }).then(() => true, () => false);

const browser = await chromium.launch();
const errors = [];
async function tab(who) {
  const page = await (await browser.newContext()).newPage();
  page.on('pageerror', (error) => errors.push(String(error)));
  await page.goto(`${base}/#/notifications`);
  if (who) await page.getByLabel('Connect as').fill(who);
  return page;
}

const { server, framework } = await (await fetch(`${base}/api/sandbox`)).json();
console.log(`${base}: ${server} / ${framework}`);
const a = await tab();
check(await shows(a.locator('.chanx-notifications[data-state="open"]')), 'the notification feed connects');
check((await a.locator('.brand').innerText()).includes(`${server} · ${framework}`), 'the sidebar names the server');
const nav = await a.locator('nav').innerText();
check(['AG-UI', 'Speech to text', 'Text to speech', 'Voice agent'].every((demo) => nav.includes(demo)), 'every demo is listed');

if (notify.length) {
  const [command, ...args] = notify;
  const title = `browser ${Date.now()}`;
  execFileSync(command, [...args, '--user', 'chris', title], { stdio: 'pipe' });
  check(await shows(a.getByText(title)), 'a notify shows in the feed');
}

await a.goto(`${base}/#/room`);
const b = await tab('bea');
await b.goto(`${base}/#/room`);
for (const page of [a, b]) await shows(page.locator('.chanx-chat[data-state="open"]'));
check(await shows(a.locator('.chanx-presence__member', { hasText: 'Bea' })), 'the roster shows the other tab');
const text = `hello ${Date.now()}`;
await a.locator('.chanx-chat__composer input').fill(text);
await a.locator('.chanx-chat__composer button').click();
const entry = b.locator('.chanx-chat__entry', { hasText: text });
check(await shows(entry), 'a message reaches the other tab');
check((await entry.locator('.chanx-chat__author').innerText()) === 'Chris', 'with its author');
await b.context().close();
check(await shows(a.locator('.chanx-presence__member', { hasText: 'Bea' }), 'detached'), 'closing a tab leaves the roster');
await a.goto(`${base}/#/agent`);
await shows(a.locator('.status[data-state="open"]'));
await a.locator('textarea').fill('hello agent');
await a.getByRole('button', { name: /run/i }).first().click();
check(await shows(a.getByText('You said')), 'the agent streams a reply over AG-UI');
check(errors.length === 0, `no page errors${errors.length ? `: ${errors.join('; ')}` : ''}`);

await browser.close();
process.exit(failed ? 1 : 0);
