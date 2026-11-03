// The JS kits behind the chanx-kit sandbox UI, on the framework of your choice:
//   npm --prefix js run sandbox -- --framework fastify
import { parseArgs } from 'node:util';

import { createSandbox, redisUrl } from './app';
import { frameworks } from './frameworks';

const { values } = parseArgs({
  options: {
    framework: { type: 'string', default: 'node' },
    port: { type: 'string', default: '8000' },
    ui: { type: 'string', default: new URL('../../sandbox/ui/dist', import.meta.url).pathname },
  },
});

const framework = values.framework as keyof typeof frameworks;
const serve = frameworks[framework];
if (!serve) throw new Error(`unknown framework ${framework}; one of ${Object.keys(frameworks)}`);

const redis = redisUrl();
await serve({
  chanx: createSandbox(redis),
  port: Number(values.port),
  ui: values.ui,
  about: { server: 'js', framework },
});
console.log(
  `chanx-kit JS sandbox on :${values.port} with ${framework}, channel layer ${redis ? `redis (${redis})` : 'in-memory'}`,
);
