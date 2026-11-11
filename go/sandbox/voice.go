package main

import (
	"os"

	chanx "go.chanx.dev/v2"

	"go.chanx.dev/kit/kits/agui"
	"go.chanx.dev/kit/kits/audiostreamin"
	"go.chanx.dev/kit/kits/audiostreamout"
	"go.chanx.dev/kit/kits/deepgram"
	"go.chanx.dev/kit/kits/elevenlabs"
	"go.chanx.dev/kit/kits/fakevoice"
	"go.chanx.dev/kit/kits/voiceagent"
)

// providers names what the voice demos run on, for the UI to show.
var providers = map[string]string{}

// speech is a real provider when its key is set, the fake otherwise; the client cannot tell.
func speech() (audiostreamin.TranscriberTopic, audiostreamout.SynthesizerTopic) {
	stt, tts := fakevoice.DefaultTranscriber.Topic(), fakevoice.DefaultSynthesizer.Topic()
	providers["stt"], providers["tts"] = "fake-voice", "fake-voice"
	if os.Getenv("DEEPGRAM_API_KEY") != "" {
		stt, providers["stt"] = deepgram.Transcriber{}.Topic(), "deepgram"
	}
	if os.Getenv("ELEVENLABS_API_KEY") != "" {
		tts, providers["tts"] = elevenlabs.Synthesizer{}.Topic(), "elevenlabs"
	}
	return stt, tts
}

// VoiceConsumer carries a session's audio in, its transcript out, and speech.
type VoiceConsumer struct {
	demoAuth
	stt audiostreamin.TranscriberTopic
	tts audiostreamout.SynthesizerTopic
}

func (c VoiceConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{c.stt, audiostreamin.TranscriptTopic{}, c.tts}
}

// AssistantConsumer composes three kits on one session: transcriber, agent, synthesizer.
type AssistantConsumer struct {
	demoAuth
	agent *voiceagent.Agent
	stt   audiostreamin.TranscriberTopic
}

func (c AssistantConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{c.agent.TranscriberTopic(c.stt), c.agent.AgUiTopic(), c.agent.Synthesizer}
}

func mountVoice(engine *chanx.Engine) {
	stt, tts := speech()
	// A voice agent with the sandbox's scripted reply.
	assistant := &voiceagent.Agent{Agent: agui.AgUiTopic{RunAgent: cannedReply}, Synthesizer: tts}
	// Audio frames must be handled in order.
	engine.Handle("/ws/voice", VoiceConsumer{stt: stt, tts: tts}, chanx.Name("voice"), chanx.Sequential(),
		chanx.Description("Speech in and out: transcripts from audio, audio from text."))
	engine.Handle("/ws/assistant", AssistantConsumer{agent: assistant, stt: stt}, chanx.Name("assistant"), chanx.Sequential(),
		chanx.Description("A voice agent: speak, hear the reply, interrupt by speaking."))
	assistant.Publisher = engine.Publisher()
}
