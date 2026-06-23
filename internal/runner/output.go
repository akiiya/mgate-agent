package runner

import (
	"bytes"
	"sync"
)

type outputLimiter struct {
	mu        sync.Mutex
	remaining int
	truncated bool
}

type limitedStream struct {
	limiter *outputLimiter
	buf     bytes.Buffer
}

func newOutputStreams(maxBytes int) (*limitedStream, *limitedStream, *outputLimiter) {
	if maxBytes <= 0 {
		maxBytes = 1
	}
	limiter := &outputLimiter{remaining: maxBytes}
	return &limitedStream{limiter: limiter}, &limitedStream{limiter: limiter}, limiter
}

func (s *limitedStream) Write(p []byte) (int, error) {
	s.limiter.mu.Lock()
	defer s.limiter.mu.Unlock()

	if s.limiter.remaining <= 0 {
		s.limiter.truncated = true
		return len(p), nil
	}
	n := len(p)
	if n > s.limiter.remaining {
		n = s.limiter.remaining
		s.limiter.truncated = true
	}
	_, _ = s.buf.Write(p[:n])
	s.limiter.remaining -= n
	return len(p), nil
}

func (s *limitedStream) String() string {
	s.limiter.mu.Lock()
	defer s.limiter.mu.Unlock()
	return s.buf.String()
}

func (l *outputLimiter) Truncated() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.truncated
}
