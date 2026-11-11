# deepgram

[Deepgram](https://deepgram.com): live speech-to-text (Nova) and streaming text-to-speech
(Aura), each behind its contract, so any transcriber or player UI works with it and
switching provider is one line.

```bash
copit add @chanx-kit-go/deepgram              # both parts
copit add @chanx-kit-go/deepgram --only stt   # speech-to-text only
copit add @chanx-kit-go/deepgram --only tts   # text-to-speech only
```

Set `DEEPGRAM_API_KEY` in the server's environment; the browser never sees it. Without a
key the topics report `provider_unavailable` rather than failing at start.

## Use it

```go
func (VoiceConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{
		deepgram.Transcriber{Options: map[string]string{"smart_format": "true"}}.Topic(),
		audiostreamin.TranscriptTopic{},
		deepgram.Synthesizer{Voice: "aura-2-thalia-en"}.Topic(),
	}
}

engine.Handle("/ws/voice", VoiceConsumer{})
```

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

A provider that does not answer within 10 s is reported as `provider_unavailable`
rather than leaving the utterance waiting; `ConnectTimeout` changes it.
