// Package voiceagent is a voice agent: audio in, transcriber, an AG-UI agent, synthesizer, audio out.
package voiceagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"iter"
	"log/slog"
	"strings"
	"sync"

	chanx "go.chanx.dev/v2"

	"go.chanx.dev/kit/kits/agui"
	"go.chanx.dev/kit/kits/audiostreamin"
	"go.chanx.dev/kit/kits/audiostreamout"
)

// Agent speaks each reply sentence as it completes, and lets speech interrupt it.
type Agent struct {
	// Agent is the AG-UI topic to run, with RunAgent or RunEvents set.
	Agent       agui.AgUiTopic
	Synthesizer audiostreamout.SynthesizerTopic
	Publisher   *chanx.Publisher
	Memory      Memory
	// OwnHistory is for a provider that keeps its own, such as pydantic-ai-ag-ui: only the new turn is sent.
	OwnHistory       bool
	MinSentenceChars int
	// MsPerChar trims a reply cut off before its audio was complete.
	MsPerChar int
	// KeepTalking turns off interrupting the agent by speaking over it.
	KeepTalking bool
	// OnTurn receives what the user finished saying; it responds by default.
	OnTurn func(ctx context.Context, session, text string) error

	mu       sync.Mutex
	spoken   map[string][]utterance
	reply    map[string]string
	finals   map[string][]string
	defaults sync.Once
}

type utterance struct{ id, text string }

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (a *Agent) init() {
	a.defaults.Do(func() {
		if a.Memory == nil {
			a.Memory = NewInMemoryMemory()
		}
		if a.MinSentenceChars == 0 {
			a.MinSentenceChars = 12
		}
		if a.MsPerChar == 0 {
			a.MsPerChar = 65
		}
		a.spoken, a.reply, a.finals = map[string][]utterance{}, map[string]string{}, map[string][]string{}
	})
}

// AgUiTopic is the agent's topic: broadcast, so every tab on the session hears the run.
func (a *Agent) AgUiTopic() agui.AgUiTopic {
	a.init()
	topic := a.Agent
	topic.Broadcast = true
	topic.RunAgent, topic.RunEvents = nil, a.speak
	return topic
}

// TranscriberTopic turns utterances into agent runs and speech over the agent into interruptions.
func (a *Agent) TranscriberTopic(transcriber audiostreamin.TranscriberTopic) audiostreamin.TranscriberTopic {
	a.init()
	transcriber.OnSpeechStarted = func(ctx context.Context, s audiostreamin.Scope, _ audiostreamin.SpeechStarted) error {
		if !a.KeepTalking {
			_, err := a.Interrupt(ctx, s.Session)
			return err
		}
		return nil
	}
	transcriber.OnFinal = func(_ context.Context, s audiostreamin.Scope, f audiostreamin.TranscriptFinal) error {
		a.mu.Lock()
		a.finals[s.Session] = append(a.finals[s.Session], f.Text)
		a.mu.Unlock()
		return nil
	}
	transcriber.OnUtteranceEnd = func(ctx context.Context, s audiostreamin.Scope, _ audiostreamin.UtteranceEnd) error {
		a.mu.Lock()
		text := strings.TrimSpace(strings.Join(a.finals[s.Session], " "))
		delete(a.finals, s.Session)
		a.mu.Unlock()
		if text == "" {
			return nil
		}
		if a.OnTurn != nil {
			return a.OnTurn(ctx, s.Session, text)
		}
		_, err := a.Respond(ctx, s.Session, text)
		return err
	}
	return transcriber
}

func (a *Agent) speak(ctx context.Context, in agui.RunAgentInput) iter.Seq2[agui.Event, error] {
	inner := a.Agent
	return func(yield func(agui.Event, error) bool) {
		session := in.ThreadID
		buffer := SentenceBuffer{MinChars: a.MinSentenceChars}
		a.mu.Lock()
		a.spoken[session] = nil
		delete(a.reply, session)
		a.mu.Unlock()
		var reply strings.Builder
		replyID := newID()
		say := func(text string) {
			id, err := a.Synthesizer.Speak(ctx, a.Publisher, session, text, "", "")
			if err != nil {
				slog.Error("voice agent could not speak", "error", err)
				return
			}
			a.mu.Lock()
			a.spoken[session] = append(a.spoken[session], utterance{id, text})
			a.mu.Unlock()
		}

		for event, err := range inner.Events(ctx, in) {
			if !yield(event, err) || err != nil {
				return
			}
			switch event.Type() {
			case "TEXT_MESSAGE_START":
				if id, ok := event["messageId"].(string); ok {
					replyID = id
				}
			case "TEXT_MESSAGE_CONTENT":
				delta, _ := event["delta"].(string)
				reply.WriteString(delta)
				for _, sentence := range buffer.Feed(delta) {
					say(sentence)
				}
			case "TEXT_MESSAGE_END":
				if rest := buffer.Flush(); rest != "" {
					say(rest)
				}
			}
		}
		// Only an uninterrupted run gets here; Interrupt keeps what was heard instead.
		if text := strings.TrimSpace(reply.String()); text != "" && ctx.Err() == nil {
			_, _ = a.remember(ctx, session, Turn{"id": replyID, "role": "assistant", "content": text})
			a.mu.Lock()
			a.reply[session] = replyID
			a.mu.Unlock()
		}
	}
}

// Respond runs the agent on the conversation plus text; "" if the session is busy.
func (a *Agent) Respond(ctx context.Context, session, text string) (string, error) {
	a.init()
	user := Turn{"id": newID(), "role": "user", "content": text}
	turns, err := a.remember(ctx, session, user)
	if err != nil {
		return "", err
	}
	if a.OwnHistory {
		turns = []Turn{user}
	}
	in := agui.RunAgentInput{
		RunID: newID(), State: map[string]any{}, Messages: turns,
		Tools: []map[string]any{}, Context: []map[string]any{}, ForwardedProps: map[string]any{},
	}
	runID, err := a.AgUiTopic().StartRun(ctx, a.Publisher, session, in)
	var busy *agui.ThreadBusyError
	if errors.As(err, &busy) {
		return "", nil
	}
	return runID, err
}

// Interrupt stops speech and the run, keeping what was heard; false if nothing was going on.
func (a *Agent) Interrupt(ctx context.Context, session string) (bool, error) {
	a.init()
	a.mu.Lock()
	spoken := a.spoken[session]
	delete(a.spoken, session)
	a.mu.Unlock()
	if !a.Agent.Running(session) && !a.Synthesizer.IsSpeaking(session) {
		return false, nil
	}
	if err := a.Synthesizer.Clear(ctx, a.Publisher, session); err != nil {
		return true, err
	}
	a.Agent.CancelRun(session, "")

	heard := a.heard(ctx, session, spoken)
	turns, err := a.Memory.Load(ctx, session)
	if err != nil {
		return true, err
	}
	a.mu.Lock()
	replyID := a.reply[session]
	delete(a.reply, session)
	a.mu.Unlock()
	// A finished reply was remembered whole; keep only what was heard.
	kept := turns[:0]
	for _, turn := range turns {
		if replyID == "" || turn["id"] != replyID {
			kept = append(kept, turn)
		}
	}
	if heard != "" {
		if replyID == "" {
			replyID = newID()
		}
		kept = append(kept, Turn{"id": replyID, "role": "assistant", "content": heard})
	}
	if err := a.Memory.Save(ctx, session, kept); err != nil {
		return true, err
	}
	event := agui.Event{"type": "CUSTOM", "name": "voice_interrupted", "value": map[string]any{"heard": heard}}
	return true, a.AgUiTopic().EmitToThread(ctx, a.Publisher, session, event)
}

func (a *Agent) heard(ctx context.Context, session string, spoken []utterance) string {
	var parts []string
	for _, u := range spoken {
		played, err := a.Synthesizer.Playback().Get(ctx, session, u.id)
		if err != nil || played == nil || played.PlayedMs <= 0 {
			break
		}
		duration := len([]rune(u.text)) * a.MsPerChar
		if played.Finished {
			duration = played.DurationMs
		}
		if played.PlayedMs >= duration {
			parts = append(parts, u.text)
			continue
		}
		if part := audiostreamout.HeardText(u.text, duration, played.PlayedMs); part != "" {
			parts = append(parts, part)
		}
		break
	}
	return strings.Join(parts, " ")
}

func (a *Agent) remember(ctx context.Context, session string, turn Turn) ([]Turn, error) {
	turns, err := a.Memory.Load(ctx, session)
	if err != nil {
		return nil, err
	}
	turns = append(turns, turn)
	return turns, a.Memory.Save(ctx, session, turns)
}
