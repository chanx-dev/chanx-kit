# ag-ui

Serve the [AG-UI protocol](https://ag-ui.com) over a chanx websocket, so any AG-UI
frontend works unchanged. Speaks **ag-ui@2**, like the Python kit, so the `ag-ui` UI kit
works against it.

```bash
copit add @chanx-kit-js/ag-ui
```

## Use it

Give the topic your agent's content events, then list it on a consumer:

```ts
import { EventType } from '@ag-ui/core';

import { AgUiRunTopic, agUiTopic } from './ws-kits/ag-ui';

export const AgentTopic = agUiTopic({
  async *runAgent(input, { signal }) {
    for await (const delta of myAgent(input.messages, signal)) {
      yield { type: EventType.TEXT_MESSAGE_CONTENT, messageId: 'm1', delta };
    }
  },
});

const agent = consumer('/ws/agent', { name: 'agent', topics: [AgentTopic, AgUiRunTopic] });
```

A client subscribes to `agui:thread:<thread_id>` and sends `ag_ui_run`. `RUN_STARTED`
and `RUN_FINISHED` bracket what you yield, and a thrown error becomes `RUN_ERROR`. Pass
`runEvents` instead when your provider emits its own lifecycle.

## One run at a time

A thread runs one run at a time. A second `ag_ui_run` while one is in flight gets a
`RUN_ERROR` naming the run holding the thread, sent to the asking connection only.
`ag_ui_cancel` with the `runId` stops it, and it ends as `RUN_ERROR` "Run cancelled.".

## Several tabs on one thread

With `broadcast: true` every connection on the thread sees the run, numbered by `seq`,
and one joining part-way is replayed the run so far. Runs then outlive the tab that
started them, and the server can start one itself:

```ts
const runId = await AgentTopic.startRun(chanx, 'thread-1', { messages });
await AgentTopic.emitToThread(chanx, 'thread-1', event);
await AgUiRunTopic.emitToRun(chanx, runId, event); // from a worker, by run id
```

Without `broadcast`, a run is its socket's: leaving the thread stops it.

## Customise

| Option | Default | Purpose |
|---|---|---|
| `runAgent(input, ctx)` | none | Content events of a run |
| `runEvents(input, ctx)` | lifecycle around `runAgent` | The whole stream |
| `broadcast` | `false` | Share runs across the thread's connections |
| `transcript(threadId)` | none | What a reconnecting client is shown first |
| `runEventStore`, `activeRunStore` | in memory | Shared stores for several processes |

## Wire contract

The payload schemas are the Python kit's, in `schema.ts` (generated, do not edit);
`@ag-ui/core` validates incoming frames. A run replayed to a late joiner carries no
`seq` yet, since chanx-js cannot stamp one on a frame for a single socket.
