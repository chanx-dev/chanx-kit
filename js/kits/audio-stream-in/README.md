# audio-stream-in

Stream microphone audio in over the socket and get transcripts back, from any
speech-to-text provider. Speaks **transcriber@1**, like the Python kit, so the
`transcriber` UI kit works unchanged.

```bash
copit add @chanx-kit-js/audio-stream-in
```

## Use it

Give the topic a provider stream, list it on a consumer, and attach the app so watchers
get the transcripts too:

```ts
import { TranscriptTopic, transcriberTopic } from './ws-kits/audio-stream-in';

const transcriber = transcriberTopic({ openStream: (audio) => openMyProvider(audio) });
const voice = consumer('/ws/voice', { name: 'voice', topics: [transcriber, TranscriptTopic] });
const chanx = createChanx({ consumers: [voice] });
transcriber.attach(chanx);
```

A client subscribes to `transcribe:<session>`, is sent `audio_config`, then streams
`audio_start`, `audio_chunk` and `audio_end`. Partial and final transcripts come back,
and watchers on `transcript:<session>` see them too. `fake-voice` needs no provider.

## Limits

Providers bill per second of audio, so every stream is bounded: `maxSessionSeconds`
(300), `idleTimeoutSeconds` (15), `maxSessionsPerUser` (2). Set one to `null` to lift
it. Lost chunks are reported as `audio_gap` rather than filled with silence.

## Writing a provider

Implement `TranscriberStream`: `send`, `keepalive`, `finish`, `close` and `frames`.
Throw `ProviderUnavailableError` when the provider refuses, such as without a key. The
hooks `onSpeechStarted`, `onFinal` and `onUtteranceEnd` act on what was heard.
