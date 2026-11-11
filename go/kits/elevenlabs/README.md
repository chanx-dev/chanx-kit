# elevenlabs

[ElevenLabs](https://elevenlabs.io): realtime speech-to-text (Scribe) and streaming
text-to-speech, each behind its contract, so any transcriber or player UI works with it
and switching provider is one line.

```bash
copit add @chanx-kit-go/elevenlabs              # both parts
copit add @chanx-kit-go/elevenlabs --only stt   # speech-to-text only
copit add @chanx-kit-go/elevenlabs --only tts   # text-to-speech only
```

Set `ELEVENLABS_API_KEY` in the server's environment; the browser never sees it.

## Use it

```go
func (VoiceConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{
		elevenlabs.Transcriber{Options: map[string]string{"vad_silence_threshold_secs": "0.8"}}.Topic(),
		audiostreamin.TranscriptTopic{},
		elevenlabs.Synthesizer{Voice: "EXAVITQu4vr4xnSDxMaL"}.Topic(), // Sarah, premade
	}
}

engine.Handle("/ws/voice", VoiceConsumer{})
```

## How it maps

| Scribe | transcriber@1 |
|---|---|
| first `partial_transcript` of an utterance | `speech_started` |
| `partial_transcript` | `transcript_partial` |
| `committed_transcript` | `transcript_final`, then `utterance_end` |
| empty chunk with `commit` on `audio_end` | last words flushed, then the stream closes |

Scribe has no speech-started event, so speech is reported at an utterance's first
partial: a fraction of a second late, enough for the voice agent's barge-in.

Speech uses the `stream-input` socket as raw PCM at the output rate. Use a premade voice
on the free tier: others answer `payment_required`, reported as `provider_unavailable`
with that hint.

A provider that does not answer within 10 s is reported as `provider_unavailable`
rather than leaving the utterance waiting; `ConnectTimeout` changes it.
