# deepgram

[Deepgram](https://deepgram.com): live speech-to-text (Nova) and streaming text-to-speech
(Aura), each behind its contract, so any transcriber or player UI works with it and
switching provider is one line.

```bash
copit add @chanx-kit-js/deepgram              # both parts
copit add @chanx-kit-js/deepgram --only stt   # speech-to-text only
copit add @chanx-kit-js/deepgram --only tts   # text-to-speech only
```

Set `DEEPGRAM_API_KEY` in the server's environment; the browser never sees it. Without a
key the topics report `provider_unavailable` rather than failing at start.

## Use it

```ts
import { TranscriptTopic } from './ws-kits/audio-stream-in';
import { deepgramSynthesizerTopic } from './ws-kits/deepgram/synthesizer';
import { deepgramTranscriberTopic } from './ws-kits/deepgram/transcriber';

const transcriber = deepgramTranscriberTopic({ options: { smart_format: 'true' } });
const voice = consumer('/ws/voice', {
  name: 'voice',
  topics: [transcriber, TranscriptTopic, deepgramSynthesizerTopic({ voice: 'aura-2-thalia-en' })],
});
```

Import from the part's module, since the other may not be installed. The second argument
takes the topic's own options; `deepgramTranscriber()` gives them bare, for wrappers such
as `voice-agent`.

## How it maps

| Deepgram | transcriber@1 |
|---|---|
| interim `Results` | `transcript_partial` |
| `Results` with `is_final` | `transcript_final` |
| `speech_final`, `UtteranceEnd` | `utterance_end` |
| `SpeechStarted` (`vad_events`) | `speech_started` |
| `Finalize` then `CloseStream` on `audio_end` | last finals, then the stream closes |

Speech opens the `speak` socket with `linear16` at the output rate, sends `Speak` and
`Flush`, and streams the audio until `Flushed`. Audio travels as binary frames to
Deepgram; base64 is only on the browser leg. No SDK: the tests run against a local
server speaking the documented protocol.
