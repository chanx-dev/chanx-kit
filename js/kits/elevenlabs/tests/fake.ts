import type { AddressInfo } from 'node:net';

import type { RawData } from 'ws';
import WebSocket, { WebSocketServer } from 'ws';

export const KEY = 'test-key';

export interface Recording {
  paths: string[];
  queries: URLSearchParams[];
  audioBytes: number;
  commits: number;
  messages: Record<string, unknown>[];
}

type Handle = (socket: WebSocket, recording: Recording) => void;

/** A local ElevenLabs behind its key check; resolves to the ws:// base URL. */
export async function fakeElevenLabs(handle: Handle) {
  const recording: Recording = { paths: [], queries: [], audioBytes: 0, commits: 0, messages: [] };
  const server = new WebSocketServer({
    port: 0,
    verifyClient: ({ req }, done) => {
      if (req.headers['xi-api-key'] !== KEY) return done(false, 401, 'invalid api key');
      const url = new URL(req.url!, 'http://x');
      recording.paths.push(url.pathname);
      recording.queries.push(url.searchParams);
      done(true);
    },
  });
  server.on('connection', (socket) => handle(socket, recording));
  await new Promise((r) => server.once('listening', r));
  const { port } = server.address() as AddressInfo;
  return { url: `ws://127.0.0.1:${port}`, recording, close: () => server.close() };
}

const json = (socket: WebSocket, message: unknown) => socket.send(JSON.stringify(message));
const parse = (data: RawData) => JSON.parse(data.toString()) as Record<string, unknown>;

/** Hears "hello" after 400 ms of audio, and commits "hello world". */
export const scribe: Handle = (socket, recording) => {
  json(socket, { message_type: 'session_started', session_id: 's' });
  let received = 0;
  let spoke = false;
  socket.on('message', (data: RawData) => {
    const message = parse(data);
    const audio = Buffer.from(String(message.audio_base_64), 'base64');
    received += audio.length;
    recording.audioBytes += audio.length;
    if (!spoke && received >= (24000 * 2 * 4) / 10) {
      spoke = true;
      json(socket, { message_type: 'partial_transcript', text: 'hello' });
    }
    if (message.commit) {
      recording.commits += 1;
      if (spoke) json(socket, { message_type: 'committed_transcript', text: 'hello world' });
    }
  });
};

/** Answers the end of input with 20 ms of audio per character, in odd-sized pieces. */
export const streamInput: Handle = (socket, recording) => {
  let spoken = '';
  socket.on('message', (data: RawData) => {
    const message = parse(data);
    recording.messages.push(message);
    if (message.text !== '') {
      spoken += String(message.text);
      return;
    }
    const audio = Buffer.alloc(((spoken.trim().length * 24000) / 50) * 2, 1);
    for (let start = 0; start < audio.length; start += 3001) {
      json(socket, { audio: audio.subarray(start, start + 3001).toString('base64'), isFinal: null });
    }
    json(socket, { audio: null, isFinal: true });
  });
};
