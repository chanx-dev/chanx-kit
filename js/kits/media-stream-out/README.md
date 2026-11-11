# media-stream-out

The plumbing for media the server streams out: producers run one at a time per session
and can be cut off as a whole, and what was sent is kept so a connection joining
part-way is replayed. The speech output kit builds on it.

```bash
copit add @chanx-kit-js/media-stream-out
```

## Use it

```ts
import { InMemoryReplayStore, SerialJobs } from './ws-kits/media-stream-out';

const jobs = new SerialJobs();
jobs.submit('session-1', (signal) => speak(text, signal));
await jobs.cancel('session-1'); // the running job is aborted, the queued ones dropped

const store = new InMemoryReplayStore();
const seq = await store.append('session-1', message); // replay to late joiners in order
```

`InMemoryReplayStore` is process-local; implement `ReplayStore` against Redis to share
it across servers.
