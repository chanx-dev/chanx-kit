// Send a notification from another process. Needs the server's REDIS_URL.
//   REDIS_URL=redis://localhost:6399/0 npm run notify -- --user chris "Hi"
import { parseArgs } from 'node:util';

import { notify } from './app';

const { values, positionals } = parseArgs({
  allowPositionals: true,
  options: { user: { type: 'string' } },
});
if (!process.env.REDIS_URL) {
  throw new Error('REDIS_URL is not set: an in-memory layer cannot reach the server.');
}

const title = positionals.join(' ') || 'Hello from another process';
await notify(title, values.user);
console.log(`Sent ${JSON.stringify(title)} to ${values.user ?? 'everyone'}`);
process.exit(0);
