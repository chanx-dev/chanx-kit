# audio-stream-out

Speak text to a session's listeners from any text-to-speech provider. Speaks
**synthesizer@1**, like the Python kit, so the `player` UI kit works unchanged.
Utterances queue in order, an interruption clears them, late joiners are replayed, and
playback marks record what was actually heard.

```bash
copit add @chanx-kit-js/audio-stream-out
```

## Use it

```ts
import { synthesizerTopic } from './ws-kits/audio-stream-out';

const voice = synthesizerTopic({ synthesize: mySynthesize });
const speech = consumer('/ws/voice', { name: 'voice', topics: [voice] });
```

A client subscribes to `speak:<session>` and sends `speak`; everyone on the session gets
`audio_start`, `audio_chunk` and `audio_end`. From the server:

```ts
await voice.speak(chanx, 'session-1', 'Your build finished.');
await voice.clear(chanx, 'session-1'); // an interruption
```

Two synthesizers on one server need different `name`s.

## Writing a provider

`synthesize(text, voice, format, signal)` is an async iterable of PCM16 audio in the
requested format; stop when `signal` aborts. Throw `SynthesizerUnavailableError` when the
provider refuses.
