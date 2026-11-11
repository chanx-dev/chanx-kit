// Package openai puts OpenAI behind the voice contracts: realtime transcription, streamed speech.
package openai

import (
	"os"
	"time"
)

// PCMRate is the only rate OpenAI realtime takes and its speech produces, as PCM16 mono.
const PCMRate = 24000

// ConnectTimeout bounds opening a connection, so a stalled provider is reported.
var ConnectTimeout = 10 * time.Second

// key is apiKey, or OPENAI_API_KEY when used, so a .env loaded after start works.
func key(apiKey string) string {
	if apiKey != "" {
		return apiKey
	}
	return os.Getenv("OPENAI_API_KEY")
}

func or[T comparable](value, fallback T) T {
	var zero T
	if value == zero {
		return fallback
	}
	return value
}
