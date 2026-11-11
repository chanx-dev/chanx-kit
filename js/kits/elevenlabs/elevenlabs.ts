import WebSocket from 'ws';

/** Mono PCM sample rates ElevenLabs takes and produces. */
export const PCM_RATES = [8000, 16000, 22050, 24000, 44100, 48000];

export const supported = (rate: number, channels: number) => PCM_RATES.includes(rate) && channels === 1;

/** The key given, or ELEVENLABS_API_KEY when used, so a .env loaded after start works. */
export const keyOf = (apiKey?: string) => apiKey || process.env.ELEVENLABS_API_KEY || undefined;

/** Connects with the key; a refusal or a stall throws `Unavailable`. */
export function dial(url: string, apiKey: string, Unavailable: new (message: string) => Error, timeoutMs = 10_000) {
  return new Promise<WebSocket>((resolve, reject) => {
    const socket = new WebSocket(url, { headers: { 'xi-api-key': apiKey }, handshakeTimeout: timeoutMs });
    socket.once('open', () => resolve(socket));
    socket.once('unexpected-response', (_req, res) => {
      reject(new Unavailable(`ElevenLabs refused the connection (${res.statusCode}).`));
      socket.terminate();
    });
    socket.once('error', (error) =>
      reject(
        new Unavailable(
          error.message.includes('handshake has timed out')
            ? `ElevenLabs did not answer within ${timeoutMs / 1000} s.`
            : `ElevenLabs is unreachable: ${error.message}`,
        ),
      ),
    );
  });
}

export type Received = { binary: true; data: Buffer } | { binary: false; data: Record<string, unknown> };

/** A socket's messages from now until it closes; an error ends it unless `quiet()`. */
export class Inbox implements AsyncIterable<Received> {
  private queue: Received[] = [];
  private done = false;
  private failure?: Error;
  private wake?: () => void;

  constructor(
    socket: WebSocket,
    private readonly quiet: () => boolean = () => false,
  ) {
    socket.on('message', (data: Buffer, binary: boolean) => {
      this.queue.push(binary ? { binary, data } : { binary, data: JSON.parse(data.toString()) });
      this.wake?.();
    });
    socket.once('close', () => this.end());
    socket.once('error', (error) => this.end(error));
  }

  async *[Symbol.asyncIterator]() {
    for (;;) {
      while (this.queue.length === 0) {
        if (this.done) {
          if (this.failure && !this.quiet()) throw this.failure;
          return;
        }
        await new Promise<void>((r) => (this.wake = r));
      }
      yield this.queue.shift()!;
    }
  }

  private end(error?: Error) {
    this.failure ??= error;
    this.done = true;
    this.wake?.();
  }
}

/** Sends unless the socket has gone. */
export function sendJson(socket: WebSocket, message: Record<string, unknown>) {
  if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify(message));
}
