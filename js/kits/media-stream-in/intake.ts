export type Quiet = 'keepalive' | 'idle';

export interface MediaIntakeOptions {
  onEnd: () => Promise<void> | void;
  onQuiet?: (quiet: Quiet) => Promise<void> | void;
  maxChunks?: number;
  /** Seconds; undefined turns it off. */
  keepaliveSeconds?: number;
  idleSeconds?: number;
}

const END = Symbol('end');

/** Feeds chunks to `sink` in order from one loop; drops the oldest when full. */
export class MediaIntake<T = Buffer> {
  dropped = 0;
  private queue: (T | typeof END)[] = [];
  private wake?: () => void;
  private stopped = false;
  private task?: Promise<void>;
  private done = false;
  private quiet = false;
  private readonly maxChunks: number;

  constructor(
    private readonly sink: (chunk: T) => Promise<void> | void,
    private readonly options: MediaIntakeOptions,
  ) {
    this.maxChunks = options.maxChunks ?? 50;
  }

  start() {
    this.task = this.run();
  }

  get running() {
    return this.task !== undefined && !this.done;
  }

  feed(chunk: T) {
    this.makeRoom();
    this.queue.push(chunk);
    this.wake?.();
  }

  /** Finish after the chunks already queued. */
  end() {
    this.makeRoom();
    this.queue.push(END);
    this.wake?.();
  }

  /** Stop now, dropping whatever is queued. Safe from inside `onQuiet`. */
  async stop() {
    this.stopped = true;
    this.queue = [];
    this.wake?.();
    if (!this.quiet) await this.task;
  }

  async wait() {
    await this.task;
  }

  private makeRoom() {
    if (this.queue.length >= this.maxChunks) {
      this.queue.shift();
      this.dropped += 1;
      console.warn(`media intake full; dropped the oldest chunk (${this.dropped} so far)`);
    }
  }

  private next(timeoutMs: number | undefined): Promise<boolean> {
    if (this.queue.length > 0 || this.stopped) return Promise.resolve(true);
    return new Promise((resolve) => {
      const timer = timeoutMs === undefined ? undefined : setTimeout(() => resolve(false), timeoutMs);
      this.wake = () => {
        clearTimeout(timer);
        this.wake = undefined;
        resolve(true);
      };
    });
  }

  private async run() {
    const { keepaliveSeconds, idleSeconds, onQuiet } = this.options;
    let lastChunk = Date.now();
    try {
      while (!this.stopped) {
        let timeoutMs = keepaliveSeconds === undefined ? undefined : keepaliveSeconds * 1000;
        if (idleSeconds !== undefined) {
          const remaining = idleSeconds * 1000 - (Date.now() - lastChunk);
          timeoutMs = Math.max(0, timeoutMs === undefined ? remaining : Math.min(timeoutMs, remaining));
        }
        if (!(await this.next(timeoutMs))) {
          const idle = idleSeconds !== undefined && Date.now() - lastChunk >= idleSeconds * 1000;
          this.quiet = true;
          try {
            await onQuiet?.(idle ? 'idle' : 'keepalive');
          } finally {
            this.quiet = false;
          }
          if (idle) return;
          continue;
        }
        if (this.stopped) return;
        const chunk = this.queue.shift()!;
        if (chunk === END) {
          await this.options.onEnd();
          return;
        }
        lastChunk = Date.now();
        await this.sink(chunk);
      }
    } finally {
      this.done = true;
    }
  }
}
