---
description: "Guide: live captions, text to speech and a voice agent you can interrupt, with swappable providers."
---

# Add voice

Speech in and speech out over your chanx websocket: microphone audio becomes live
captions, text becomes audio every listener hears, and optionally an agent answers out
loud. Provider API keys stay on your server, and switching provider is one line.

The pieces:

| Kit | Side | What it does |
|---|---|---|
| [`fake-voice`](kits/fake-voice.md), [`deepgram`](kits/deepgram.md), [`elevenlabs`](kits/elevenlabs.md), [`openai`](kits/openai.md) | server | a provider, with a speech-to-text part (`stt`) and a text-to-speech part (`tts`) |
| [`audio-stream-in`](kits/audio-stream-in.md), [`audio-stream-out`](kits/audio-stream-out.md) | server | the contracts and plumbing; installed with a provider |
| [`transcriber`](ui/transcriber.md), [`player`](ui/player.md) | browser | microphone and captions, playback |
| [`voice-agent`](kits/voice-agent.md) (both sides) | both | an AG-UI agent you talk to |

Voice runs on Node, Python and Go servers. This page assumes both registries are set up
([add kits to your app](add-to-your-app.md)); the [templates](start-a-project.md) have
them already.

## 1. Try it with no account

`fake-voice` transcribes any speech as a scripted sentence and speaks text as a tone
per word, so the whole path works before you sign up anywhere. Add it with the two UI
kits, then mount the topics on a consumer:

=== "Node"

    ```bash
    copit add @chanx-kit-js/fake-voice @chanx-kit-ui/transcriber @chanx-kit-ui/player
    ```

    ```ts
    import { TranscriptTopic } from './ws-kits/audio-stream-in';
    import { fakeSynthesizerTopic } from './ws-kits/fake-voice/synthesizer';
    import { fakeTranscriberTopic } from './ws-kits/fake-voice/transcriber';

    const transcriber = fakeTranscriberTopic();
    export const voice = consumer('/ws/voice', {
      name: 'voice',
      topics: [transcriber, TranscriptTopic, fakeSynthesizerTopic()],
    });

    // once the app is created, so transcripts reach watchers too:
    transcriber.attach(chanx);
    ```

=== "Python"

    ```bash
    copit add @chanx-kit/fake-voice @chanx-kit-ui/transcriber @chanx-kit-ui/player
    ```

    ```python
    from chanx.core.decorators import channel
    from chanx.fast_channels.websocket import AsyncJsonWebsocketConsumer

    from .ws_kits.audio_stream_in import TranscriptTopic
    from .ws_kits.fake_voice.synthesizer import FakeSynthesizerTopic
    from .ws_kits.fake_voice.transcriber import FakeTranscriberTopic


    class TranscriberTopic(FakeTranscriberTopic):
        async def authorize(self, **params: str) -> bool:
            return user_owns_session(self.scope, params["session"])


    class SynthesizerTopic(FakeSynthesizerTopic):
        pass


    @channel(name="voice")
    class VoiceConsumer(AsyncJsonWebsocketConsumer):
        channel_layer_alias = "default"
        topics = [TranscriberTopic, TranscriptTopic, SynthesizerTopic]
    ```

=== "Go"

    ```bash
    copit add @chanx-kit-go/fake-voice @chanx-kit-ui/transcriber @chanx-kit-ui/player
    ```

    ```go
    type VoiceConsumer struct{}

    func (VoiceConsumer) Topics() []chanx.TopicHandler {
    	return []chanx.TopicHandler{
    		fakevoice.DefaultTranscriber.Topic(),
    		audiostreamin.TranscriptTopic{},
    		fakevoice.DefaultSynthesizer.Topic(),
    	}
    }

    engine.Handle("/ws/voice", VoiceConsumer{}, chanx.Name("voice"))
    ```

Route it at `/ws/voice`, regenerate the client (`npm --prefix web run gen`), and use it:

```tsx
import { Recorder, TranscriptView, useTranscriber } from '@/chanx-kit/transcriber/react';
import { Player, usePlayer } from '@/chanx-kit/player/react';
import '@/chanx-kit/transcriber/transcriber.css';
import '@/chanx-kit/player/player.css';
import { voice } from '@/generated';

export function Voice({ session }: { session: string }) {
  const mic = useTranscriber(voice, voice.topics.transcriberTopic.with({ session }));
  const speech = usePlayer(voice, voice.topics.synthesizerTopic.with({ session }));

  return (
    <>
      <Recorder
        recording={mic.recording}
        level={mic.level}
        error={mic.error}
        onStart={mic.start}
        onStop={mic.stop}
        disabled={mic.status !== 'open'}
      />
      <TranscriptView utterances={mic.utterances} speaking={mic.speaking} />
      <Player {...speech} onUnlock={speech.unlock} onStop={speech.stop} />
      <button onClick={() => { void speech.unlock(); speech.speak('Hello there.'); }}>
        Speak
      </button>
    </>
  );
}
```

Start recording from a click: browsers only open the microphone and play audio after a
user gesture.

## 2. Switch to a real provider

Install the provider and swap the two topics. The browser code does not change.

=== "Node"

    ```bash
    copit add @chanx-kit-js/deepgram
    ```

    ```ts
    import { deepgramSynthesizerTopic } from './ws-kits/deepgram/synthesizer';
    import { deepgramTranscriberTopic } from './ws-kits/deepgram/transcriber';

    const transcriber = deepgramTranscriberTopic();
    // topics: [transcriber, TranscriptTopic, deepgramSynthesizerTopic()]
    ```

=== "Python"

    ```bash
    copit add @chanx-kit/deepgram
    ```

    ```python
    from .ws_kits.deepgram.synthesizer import DeepgramSynthesizerTopic
    from .ws_kits.deepgram.transcriber import DeepgramTranscriberTopic


    class TranscriberTopic(DeepgramTranscriberTopic): ...
    class SynthesizerTopic(DeepgramSynthesizerTopic): ...
    ```

=== "Go"

    ```bash
    copit add @chanx-kit-go/deepgram
    ```

    ```go
    return []chanx.TopicHandler{
    	deepgram.Transcriber{}.Topic(),
    	audiostreamin.TranscriptTopic{},
    	deepgram.Synthesizer{}.Topic(),
    }
    ```

Set the key in the server's environment. It is read when used, so a `.env` loaded at
startup works:

| Provider | Key | Speech to text | Text to speech |
|---|---|---|---|
| `deepgram` | `DEEPGRAM_API_KEY` | Nova, live | Aura |
| `elevenlabs` | `ELEVENLABS_API_KEY` | Scribe realtime | streaming voices (premade ones on the free tier) |
| `openai` | `OPENAI_API_KEY` | realtime transcription | `gpt-4o-mini-tts` |

### Only one side, or two providers

Each provider installs both parts by default. Keep one with `--only`, which also
leaves out the audio kit only the other part needs. It applies to the kits that have
that part, so the UI kit can come in the same command:

```bash
copit add @chanx-kit/deepgram @chanx-kit-ui/transcriber --only stt   # captions
copit add @chanx-kit/elevenlabs @chanx-kit-ui/player --only tts      # ElevenLabs speaks
```

On Node and Go, use `@chanx-kit-js/` or `@chanx-kit-go/` and import from the part's
module (`deepgram/transcriber`), since the other part may not be installed.

Two providers each with their own part take two commands, as above. Mixing registries
with `--only` needs copit 0.9.1.

## 3. Talk to an agent

The voice-agent kit connects the transcriber to an [AG-UI](kits/ag-ui.md) agent and
the agent's reply to the synthesizer: each utterance starts a run once the speaker
pauses, the reply is spoken sentence by sentence while it streams, and speaking over
it interrupts.

=== "Node"

    ```bash
    copit add @chanx-kit-js/voice-agent @chanx-kit-ui/voice-agent
    ```

    ```ts
    import { deepgramTranscriber } from './ws-kits/deepgram/transcriber';
    import { deepgramSynthesizerTopic } from './ws-kits/deepgram/synthesizer';
    import { voiceAgent } from './ws-kits/voice-agent';

    const assistant = voiceAgent({
      synthesizer: deepgramSynthesizerTopic(),
      async *runAgent(input, { signal }) {
        yield* myAgent(input.messages, signal);
      },
    });

    export const assistantConsumer = consumer('/ws/assistant', {
      name: 'assistant',
      topics: [assistant.transcriberTopic(deepgramTranscriber()), assistant.topic, assistant.synthesizer],
    });

    // once the app is created:
    assistant.attach(chanx);
    ```

=== "Python"

    ```bash
    copit add @chanx-kit/voice-agent @chanx-kit-ui/voice-agent
    ```

    ```python
    from .ws_kits import voice_agent


    class AgUiTopic(voice_agent.VoiceAgentTopic):
        synthesizer = SynthesizerTopic

        async def run_agent(self, run_input):
            async for event in my_agent(run_input.messages):
                yield event


    class TranscriberTopic(voice_agent.VoiceTranscriberTopic, DeepgramTranscriberTopic):
        agent_topic = AgUiTopic


    @channel(name="assistant")
    class AssistantConsumer(AsyncJsonWebsocketConsumer):
        channel_layer_alias = "default"
        topics = [TranscriberTopic, AgUiTopic, SynthesizerTopic]
    ```

=== "Go"

    ```bash
    copit add @chanx-kit-go/voice-agent @chanx-kit-ui/voice-agent
    ```

    ```go
    var assistant = &voiceagent.Agent{
    	Agent:       agui.AgUiTopic{RunAgent: runAgent},
    	Synthesizer: deepgram.Synthesizer{}.Topic(),
    }

    type AssistantConsumer struct{}

    func (AssistantConsumer) Topics() []chanx.TopicHandler {
    	return []chanx.TopicHandler{
    		assistant.TranscriberTopic(deepgram.Transcriber{}.Topic()),
    		assistant.AgUiTopic(),
    		assistant.Synthesizer,
    	}
    }

    engine.Handle("/ws/assistant", AssistantConsumer{}, chanx.Name("assistant"))
    assistant.Publisher = engine.Publisher()
    ```

One session id names all three topics, so the browser hooks them together:

```tsx
import { VoiceAgent, useVoiceAgent } from '@/chanx-kit/voice-agent/react';

const agent = useVoiceAgent(assistant, {
  transcriber: assistant.topics.transcriberTopic.with({ session }),
  synthesizer: assistant.topics.synthesizerTopic.with({ session }),
  thread: assistant.topics.agUiTopic.with({ thread_id: session }),
});
return <VoiceAgent agent={agent} />;
```

Barge-in needs the transcriber to report speech starting: Deepgram and ElevenLabs do,
OpenAI does with server VAD turn detection and `gpt-4o-transcribe` (see
[`openai`](kits/openai.md)). Without headphones, the browser's echo cancellation keeps the
agent from hearing itself.

## Good to know

- **Limits.** Providers bill per second of audio, so transcriber streams are bounded:
  by session length, idle time and streams per user, each one setting on the topic
  (see [`audio-stream-in`](kits/audio-stream-in.md#limits)). Text to speak is capped
  too.
- **Several processes.** Replay for late joiners, playback marks, the agent's memory
  and interruption live in the process holding the socket. Implement the kits' store
  protocols against Redis before running more than one worker.
- **Other providers.** On Python, [`pipecat-bridge`](kits/pipecat-bridge.md) puts any
  [Pipecat](https://docs.pipecat.ai) speech-to-text service behind the same contract.
- **Your own provider.** Implement the small transcriber stream or synthesize function
  the provider kits do; see [`audio-stream-in`](kits/audio-stream-in.md#writing-a-provider).
