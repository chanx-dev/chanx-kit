# openai

OpenAI: realtime transcription and streamed text-to-speech, each behind its contract, so
any transcriber or player UI works with it and switching provider is one line.

```bash
copit add @chanx-kit-js/openai              # both parts
copit add @chanx-kit-js/openai --only stt   # speech-to-text only
copit add @chanx-kit-js/openai --only tts   # text-to-speech only
```

Set `OPENAI_API_KEY` in the server's environment; the browser never sees it. Both take
and produce PCM16 mono at 24 kHz, the kits' default format.

## Use it

```ts
import { TranscriptTopic } from './ws-kits/audio-stream-in';
import { openAISynthesizerTopic } from './ws-kits/openai/synthesizer';
import { openAITranscriberTopic } from './ws-kits/openai/transcriber';

const voice = consumer('/ws/voice', {
  name: 'voice',
  topics: [
    openAITranscriberTopic({ model: 'gpt-4o-transcribe', turnDetection: { type: 'server_vad' } }),
    TranscriptTopic,
    openAISynthesizerTopic({ voice: 'alloy', instructions: 'Friendly and brisk.' }),
  ],
});
```

Import from the part's module, since the other may not be installed.
`openAITranscriber()` gives the transcriber options bare, for wrappers such as
`voice-agent`.

## How it maps

| OpenAI | transcriber@1 |
|---|---|
| `...input_audio_transcription.delta` | `transcript_partial` (the turn's text so far) |
| `...input_audio_transcription.completed` | `transcript_final`, then `utterance_end` |
| `input_audio_buffer.speech_started` (with `server_vad`) | `speech_started` |
| `input_audio_buffer.commit` on `audio_end` | the turn is transcribed, then the stream closes |

Without `turnDetection` one `audio_start` to `audio_end` is one turn. With `server_vad`
OpenAI splits turns on silence and reports speech starting, which the voice agent uses
for barge-in; it needs `gpt-4o-transcribe` or `gpt-4o-mini-transcribe`, since the default
`gpt-live-transcribe` refuses turn detection.

Speech posts to `/v1/audio/speech` with `response_format: pcm` and streams the body.
