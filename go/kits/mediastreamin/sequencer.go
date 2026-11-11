// Package mediastreamin takes media a client streams in; []byte travels as base64.
package mediastreamin

// Gap is chunks Start up to End (exclusive) that never arrived.
type Gap struct{ Start, End int }

// ChunkSequencer releases chunks in index order and reports ones that never arrive.
type ChunkSequencer struct {
	// Window is how many chunks wait for a missing one before it counts as lost.
	Window   int
	Expected int
	pending  map[int][]byte
}

func NewChunkSequencer(window int) *ChunkSequencer {
	return &ChunkSequencer{Window: window, pending: map[int][]byte{}}
}

func (s *ChunkSequencer) Reset() {
	s.Expected = 0
	clear(s.pending)
}

// Push returns the chunks now in order, and any gaps.
func (s *ChunkSequencer) Push(index int, data []byte) ([][]byte, []Gap) {
	if _, held := s.pending[index]; index < s.Expected || held {
		return nil, nil
	}
	s.pending[index] = data
	return s.release(false)
}

// Flush releases everything held.
func (s *ChunkSequencer) Flush() ([][]byte, []Gap) { return s.release(true) }

func (s *ChunkSequencer) release(giveUp bool) (ready [][]byte, gaps []Gap) {
	for len(s.pending) > 0 {
		if data, ok := s.pending[s.Expected]; ok {
			delete(s.pending, s.Expected)
			ready = append(ready, data)
			s.Expected++
			continue
		}
		if !giveUp && len(s.pending) <= s.Window {
			break
		}
		following := -1
		for index := range s.pending {
			if following < 0 || index < following {
				following = index
			}
		}
		gaps = append(gaps, Gap{s.Expected, following})
		s.Expected = following
	}
	return ready, gaps
}
