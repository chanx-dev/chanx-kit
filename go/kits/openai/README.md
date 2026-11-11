# openai

OpenAI: realtime transcription and streamed text-to-speech, each behind its contract, so
any transcriber or player UI works with it and switching provider is one line.

```bash
copit add @chanx-kit-go/openai              # both parts
copit add @chanx-kit-go/openai --only stt   # speech-to-text only
copit add @chanx-kit-go/openai --only tts   # text-to-speech only
```

Set `OPENAI_API_KEY` in the server's environment; the browser never sees it. Both take
and produce PCM16 mono at 24 kHz, the kits' default format.

## Use it

```go
func (VoiceConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{
		openai.Transcriber{Model: "gpt-4o-transcribe", TurnDetection: map[string]any{"type": "server_vad"}}.Topic(),
		audiostreamin.TranscriptTopic{},
		openai.Synthesizer{Voice: "alloy", Instructions: "Friendly and brisk."}.Topic(),
	}
}

engine.Handle("/ws/voice", VoiceConsumer{}, chanx.Sequential())
```

## How it maps

| OpenAI | transcriber@1 |
|---|---|
| `...input_audio_transcription.delta` | `transcript_partial` (the turn's text so far) |
| `...input_audio_transcription.completed` | `transcript_final`, then `utterance_end` |
| `input_audio_buffer.speech_started` (with `server_vad`) | `speech_started` |
| `input_audio_buffer.commit` on `audio_end` | the turn is transcribed, then the stream closes |

Without `TurnDetection` one `audio_start` to `audio_end` is one turn. With `server_vad`
OpenAI splits turns on silence and reports speech starting, which the voice agent uses
for barge-in; it needs `gpt-4o-transcribe` or `gpt-4o-mini-transcribe`, since the default
`gpt-live-transcribe` refuses turn detection.

Speech posts to `/v1/audio/speech` with `response_format: pcm` and streams the body.

A provider that does not answer within 10 s is reported as `provider_unavailable`
rather than leaving the utterance waiting; `ConnectTimeout` changes it.
