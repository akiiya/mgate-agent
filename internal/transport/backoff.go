package transport

import (
	"math/rand"
	"time"
)

type Backoff struct {
	min    time.Duration
	max    time.Duration
	jitter float64
	rand   *rand.Rand
	next   time.Duration
}

func NewBackoff(minDelay, maxDelay time.Duration, jitter float64) *Backoff {
	if minDelay <= 0 {
		minDelay = time.Second
	}
	if maxDelay < minDelay {
		maxDelay = minDelay
	}
	if jitter < 0 {
		jitter = 0
	}
	return &Backoff{
		min:    minDelay,
		max:    maxDelay,
		jitter: jitter,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (b *Backoff) Next() time.Duration {
	if b.next == 0 {
		b.next = b.min
	} else {
		b.next *= 2
		if b.next > b.max {
			b.next = b.max
		}
	}
	if b.jitter == 0 {
		return b.next
	}
	delta := float64(b.next) * b.jitter
	offset := (b.rand.Float64()*2 - 1) * delta
	delay := time.Duration(float64(b.next) + offset)
	if delay < 0 {
		return 0
	}
	return delay
}

func (b *Backoff) Reset() {
	b.next = 0
}
