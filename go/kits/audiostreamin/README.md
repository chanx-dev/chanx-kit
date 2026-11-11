# audio-stream-in

Stream microphone audio in over the socket and get transcripts back, from any
speech-to-text provider. Speaks **transcriber@1**, like the Python kit, so the
`transcriber` UI kit works unchanged.

```bash
copit add @chanx-kit-go/audio-stream-in
```

## Use it

Give the topic a provider stream, and mount its consumer with `chanx.Sequential()`, so
`audio_start`, the chunks and `audio_end` are handled in order:

```go
func (VoiceConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{
		audiostreamin.TranscriberTopic{OpenStream: deepgram.Transcriber{}.Open},
		audiostreamin.TranscriptTopic{},
	}
}

engine.Handle("/ws/voice", VoiceConsumer{}, chanx.Sequential())
```

A client subscribes to `transcribe:<session>`, is sent `audio_config`, then streams
`audio_start`, `audio_chunk` and `audio_end`. Partial and final transcripts come back,
and watchers on `transcript:<session>` see them too. `fake-voice` needs no provider.

## Limits

Providers bill per second of audio, so every stream is bounded: `MaxSession` (5 min),
`IdleTimeout` (15 s), `MaxSessionsPerUser` (2). Set one to `audiostreamin.Off` to lift
it. Lost chunks are reported as `audio_gap` rather than filled with silence.

## Writing a provider

Implement `TranscriberStream`: `Send`, `Keepalive`, `Finish`, `Close` and `Frames`. Wrap
`audiostreamin.ErrProviderUnavailable` when the provider refuses, such as without a key.
The hooks `OnSpeechStarted`, `OnFinal` and `OnUtteranceEnd` act on what was heard.
