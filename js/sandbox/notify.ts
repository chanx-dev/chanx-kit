// Push a notification from outside the server process; both need REDIS_URL.
//   npm --prefix js run sandbox:notify -- --user chris "Just for you"
import { parseArgs } from 'node:util';

import { notifyAll, notifyUser } from '../kits/notification';

import { createSandbox, redisUrl } from './app';

const { values, positionals } = parseArgs({
  allowPositionals: true,
  options: { user: { type: 'string' } },
});
const redis = redisUrl();
if (!redis) {
  throw new Error('REDIS_URL is not set: an in-memory layer cannot reach the server process');
}

const chanx = createSandbox(redis);
const title = positionals.join(' ') || 'Hello from a worker';
const payload = { title, body: 'sent from a separate process' };
await (values.user ? notifyUser(chanx, values.user, payload) : notifyAll(chanx, payload));
console.log(`Sent ${JSON.stringify(title)} to ${values.user ?? 'everyone'}`);
process.exit(0);
