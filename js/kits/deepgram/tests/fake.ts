import type { AddressInfo } from 'node:net';

import type { RawData } from 'ws';
import WebSocket, { WebSocketServer } from 'ws';

export const KEY = 'test-key';

export interface Recording {
  queries: URLSearchParams[];
  audioBytes: number;
  controls: string[];
}

type Handle = (socket: WebSocket, recording: Recording) => void;

/** A local Deepgram behind its key check; resolves to the ws:// base URL. */
export async function fakeDeepgram(handle: Handle) {
  const recording: Recording = { queries: [], audioBytes: 0, controls: [] };
  const server = new WebSocketServer({
    port: 0,
    verifyClient: ({ req }, done) => {
      if (req.headers.authorization !== `Token ${KEY}`) return done(false, 401, 'invalid credentials');
      recording.queries.push(new URL(req.url!, 'http://x').searchParams);
      done(true);
    },
  });
  server.on('connection', (socket) => handle(socket, recording));
  await new Promise((r) => server.once('listening', r));
  const { port } = server.address() as AddressInfo;
  return { url: `ws://127.0.0.1:${port}`, recording, close: () => server.close() };
}

const json = (socket: WebSocket, message: unknown) => socket.send(JSON.stringify(message));
const control = (data: RawData) => (JSON.parse(data.toString()) as { type: string; text?: string });

const results = (text: string, isFinal: boolean, start: number, duration: number) => ({
  type: 'Results',
  is_final: isFinal,
  start,
  duration,
  channel: { alternatives: [{ transcript: text, confidence: 0.93 }] },
});

/** Hears "hello" after 400 ms of audio, then "hello world" once finalized. */
export const listen: Handle = (socket, recording) => {
  let received = 0;
  let spoke = false;
  socket.on('message', (data: RawData, binary) => {
    if (binary) {
      const size = (data as Buffer).length;
      received += size;
      recording.audioBytes += size;
      if (!spoke && received >= (24000 * 2 * 4) / 10) {
        spoke = true;
        json(socket, { type: 'SpeechStarted', timestamp: 0.1 });
        json(socket, results('hello', false, 0.1, 0.3));
      }
      return;
    }
    const { type } = control(data);
    recording.controls.push(type);
    if (type === 'Finalize' && spoke) {
      json(socket, results('hello world', true, 0.1, 0.8));
      json(socket, { type: 'UtteranceEnd', last_word_end: 0.9 });
    } else if (type === 'CloseStream') {
      json(socket, { type: 'Metadata', request_id: 'r1' });
      socket.close();
    }
  });
};

/** Answers Flush with 20 ms of audio per character, in odd-sized pieces. */
export const speak: Handle = (socket, recording) => {
  json(socket, { type: 'Metadata', request_id: 'r1' });
  let text = '';
  socket.on('message', (data: RawData) => {
    const message = control(data);
    recording.controls.push(message.type);
    if (message.type === 'Speak') text += message.text;
    if (message.type === 'Flush') {
      const audio = Buffer.alloc(((text.length * 24000) / 50) * 2, 3);
      for (let start = 0; start < audio.length; start += 4001) socket.send(audio.subarray(start, start + 4001));
      json(socket, { type: 'Flushed', sequence_id: 0 });
    }
  });
};
