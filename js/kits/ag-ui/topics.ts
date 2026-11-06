import { randomUUID } from 'node:crypto';

import type { BaseEvent, RunAgentInput } from '@ag-ui/core';
import { EventType } from '@ag-ui/core';
import type { Chanx, PublishOptions } from '@chanx-js/server';
import { topic } from '@chanx-js/server';

import { AgUiCancelMessage, AgUiEventMessage, AgUiRunMessage } from './messages';
import type { ActiveRunStore, RunEventStore } from './store';
import { InMemoryActiveRunStore, InMemoryRunEventStore } from './store';

export interface RunContext {
  threadId: string;
  /** Aborts when the run is cancelled. */
  signal: AbortSignal;
}

type EventStream = (input: RunAgentInput, ctx: RunContext) => AsyncIterable<BaseEvent>;

export interface AgUiTopicOptions {
  /** The run's content events; its start and finish are added around them. */
  runAgent?: EventStream;
  /** The whole stream, for providers that emit their own lifecycle. */
  runEvents?: EventStream;
  /** Send each run to every connection on the thread, replaying it to late joiners. */
  broadcast?: boolean;
  /** What a reconnecting client is shown first, usually a MESSAGES_SNAPSHOT. */
  transcript?: (threadId: string) => Promise<BaseEvent | undefined> | BaseEvent | undefined;
  runEventStore?: RunEventStore;
  activeRunStore?: ActiveRunStore;
  /** Group namespace, shared with any server on the same layer. */
  name?: string;
}

export class ThreadBusyError extends Error {
  constructor(
    readonly threadId: string,
    readonly activeRunId: string,
  ) {
    super(`Thread ${threadId} is already running ${activeRunId}.`);
  }
}

const TERMINAL = new Set<string>([EventType.RUN_FINISHED, EventType.RUN_ERROR]);
const newRunId = () => randomUUID().replaceAll('-', '');
const runError = (message: string): BaseEvent =>
  ({ type: EventType.RUN_ERROR, message }) as BaseEvent;

type Emit = (event: BaseEvent) => Promise<void>;
type Publish = (event: BaseEvent, options?: PublishOptions) => Promise<void>;

/** Iterate `stream`, stopping as soon as `signal` aborts, even mid-await. */
async function* untilAborted<T>(stream: AsyncIterable<T>, signal: AbortSignal) {
  const iterator = stream[Symbol.asyncIterator]();
  const aborted = new Promise<never>((_, reject) =>
    signal.addEventListener('abort', () => reject(signal.reason), { once: true }),
  );
  aborted.catch(() => {});
  try {
    while (true) {
      const next = await Promise.race([iterator.next(), aborted]);
      if (next.done) return;
      yield next.value;
    }
  } finally {
    void iterator.return?.();
  }
}

/** The AG-UI protocol on `agui:thread:<thread_id>`; give it `runAgent` or `runEvents`. */
export function agUiTopic(options: AgUiTopicOptions = {}) {
  const runEventStore = options.runEventStore ?? new InMemoryRunEventStore();
  const activeRunStore = options.activeRunStore ?? new InMemoryActiveRunStore();
  const runs = new Map<string, AbortController>();
  const key = (threadId: string, runId: string) => `${threadId}\n${runId}`;

  const runEvents: EventStream =
    options.runEvents ??
    async function* (input, ctx) {
      if (!options.runAgent) throw new Error('agUiTopic needs runAgent or runEvents');
      const ids = { threadId: input.threadId, runId: input.runId };
      yield { type: EventType.RUN_STARTED, ...ids } as BaseEvent;
      yield* options.runAgent(input, ctx);
      yield { type: EventType.RUN_FINISHED, ...ids } as BaseEvent;
    };

  async function broadcastRunEvent(publish: Publish, threadId: string, event: BaseEvent) {
    if (event.type === EventType.RUN_STARTED) await runEventStore.clear(threadId);
    const seq = await runEventStore.append(threadId, event);
    await publish(event, { seq });
    if (TERMINAL.has(event.type)) await runEventStore.clear(threadId);
  }

  /** Run a claimed run to its end and release the thread, however it ends. */
  async function drive(input: RunAgentInput, emit: Emit, own?: Set<string>) {
    const id = key(input.threadId, input.runId);
    const controller = new AbortController();
    runs.set(id, controller);
    own?.add(id);
    const ctx = { threadId: input.threadId, signal: controller.signal };
    try {
      for await (const event of untilAborted(runEvents(input, ctx), controller.signal)) {
        await emit(event);
      }
    } catch (error) {
      // A cancelled run did not produce what was asked, so it ends as an error.
      await emit(
        runError(controller.signal.aborted ? 'Run cancelled.' : String((error as Error)?.message ?? error)),
      );
    } finally {
      runs.delete(id);
      own?.delete(id);
      await activeRunStore.end(input.threadId, input.runId);
    }
  }

  const built = topic('agui:thread:{thread_id}', {
    name: options.name ?? 'AgUiTopic',
    description: 'The AG-UI protocol for one thread.',
    state: () => ({ own: new Set<string>() }),
    // State first, then the run in flight: AG-UI cannot join a stream part-way through.
    onSubscribe: async (ctx) => {
      const transcript = await options.transcript?.(ctx.params.thread_id);
      if (transcript) await ctx.push(AgUiEventMessage.create(transcript));
      if (!options.broadcast) return;
      for (const [, event] of await runEventStore.replay(ctx.params.thread_id)) {
        await ctx.push(AgUiEventMessage.create(event));
      }
    },
    // A run only this socket sees is pointless once it leaves; a broadcast one is the thread's.
    onUnsubscribe: (ctx) => {
      if (options.broadcast) return;
      for (const id of ctx.state.own) runs.get(id)?.abort();
    },
  })
    .on(
      AgUiRunMessage,
      {
        output: AgUiEventMessage,
        summary: 'Run the agent',
        description: 'Run an AG-UI agent and stream its events back.',
      },
      async (m, ctx) => {
        const threadId = ctx.params.thread_id;
        const input = { ...m.payload, threadId, runId: m.payload.runId || newRunId() };
        const holder = await activeRunStore.begin(threadId, input.runId);
        if (holder !== undefined) {
          // Only the asking connection hears this; the run in flight carries on.
          await ctx.send(AgUiEventMessage.create(runError(`Thread is already running ${holder}.`)));
          return;
        }
        const emit: Emit = options.broadcast
          ? (event) =>
              broadcastRunEvent(
                (e, o) => ctx.publish(AgUiEventMessage.create(e), o),
                threadId,
                event,
              )
          : (event) => ctx.push(AgUiEventMessage.create(event));
        void drive(input, emit, ctx.state.own);
      },
    )
    .on(
      AgUiCancelMessage,
      {
        output: AgUiEventMessage,
        summary: 'Cancel the run',
        description: 'Stop the run in flight on this thread.',
      },
      (m, ctx) => {
        runs.get(key(ctx.params.thread_id, m.payload.runId))?.abort();
      },
    )
    .passthrough(AgUiEventMessage);

  const publisher =
    (chanx: Chanx, threadId: string): Publish =>
    (event, publishOptions) =>
      chanx.publish(built, built.with({ thread_id: threadId }), AgUiEventMessage.create(event), publishOptions);

  return Object.assign(built, {
    /** Run the agent on `threadId` from the server; needs `broadcast`. */
    async startRun(chanx: Chanx, threadId: string, input: Partial<RunAgentInput> = {}) {
      if (!options.broadcast) {
        throw new Error('startRun needs broadcast: no connection asked for this run');
      }
      const run = { ...input, threadId, runId: input.runId || newRunId() } as RunAgentInput;
      const holder = await activeRunStore.begin(threadId, run.runId);
      if (holder !== undefined) throw new ThreadBusyError(threadId, holder);
      const publish = publisher(chanx, threadId);
      void drive(run, (event) => broadcastRunEvent(publish, threadId, event));
      return run.runId;
    },
    /** Stop this process's run on `threadId`, any when `runId` is left out. */
    cancelRun(threadId: string, runId?: string) {
      let stopped = false;
      for (const [id, controller] of runs) {
        const [thread, run] = id.split('\n');
        if (thread === threadId && (runId === undefined || run === runId)) {
          controller.abort();
          stopped = true;
        }
      }
      return stopped;
    },
    /** Whether a run on `threadId` is in flight in this process. */
    running: (threadId: string) => [...runs.keys()].some((id) => id.startsWith(`${threadId}\n`)),
    /** Emit into a thread from outside; recorded for replay while a run is in flight. */
    async emitToThread(chanx: Chanx, threadId: string, event: BaseEvent) {
      const publish = publisher(chanx, threadId);
      const inRun = event.type === EventType.RUN_STARTED || (await runEventStore.replay(threadId)).length > 0;
      if (options.broadcast && inRun) return broadcastRunEvent(publish, threadId, event);
      return publish(event);
    },
  });
}

/** One run's events, `agui:run:<run_id>`, for work off the connection to emit into. */
export const AgUiRunTopic = Object.assign(
  topic('agui:run:{run_id}', {
    name: 'AgUiRunTopic',
    description: "One run's events.",
  }).passthrough(AgUiEventMessage),
  {
    emitToRun(chanx: Chanx, runId: string, event: BaseEvent) {
      return chanx.publish(
        AgUiRunTopic,
        AgUiRunTopic.with({ run_id: runId }),
        AgUiEventMessage.create(event),
      );
    },
  },
);
