package voiceagent

import (
	"regexp"
	"strings"
)

// A sentence ends at . ! ? or an ellipsis followed by space, or at a line break.
var boundary = regexp.MustCompile(`[.!?…]["')\]]*\s+|\n+`)

// SentenceBuffer cuts streamed text into sentences; tiny ones wait for the next.
type SentenceBuffer struct {
	MinChars int
	text     string
}

func (b *SentenceBuffer) Feed(delta string) []string {
	b.text += delta
	var sentences []string
	start := 0
	for _, match := range boundary.FindAllStringIndex(b.text, -1) {
		if candidate := strings.TrimSpace(b.text[start:match[1]]); len([]rune(candidate)) >= b.MinChars {
			sentences = append(sentences, candidate)
			start = match[1]
		}
	}
	b.text = b.text[start:]
	return sentences
}

// Flush returns whatever is left, at the end of the reply.
func (b *SentenceBuffer) Flush() string {
	rest := strings.TrimSpace(b.text)
	b.text = ""
	return rest
}
