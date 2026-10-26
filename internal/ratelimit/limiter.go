package ratelimit

import (
	"sync"
	"time"
)

// Limiter cuenta intentos en memoria por clave. No bloquea la cuenta para siempre.
type Limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func New() *Limiter {
	return &Limiter{hits: map[string][]time.Time{}}
}

func (l *Limiter) prune(key string, window time.Duration, now time.Time) []time.Time {
	cut := now.Add(-window)
	var kept []time.Time
	for _, at := range l.hits[key] {
		if at.After(cut) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = kept
	return kept
}

// Blocked indica si la clave ya llegó al tope, sin anotar un intento nuevo.
func (l *Limiter) Blocked(key string, max int, window time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key, window, now)) >= max
}

// Hit anota un intento. justTripped es true solo la vez que se alcanza el tope.
func (l *Limiter) Hit(key string, max int, window time.Duration, now time.Time) (blocked bool, justTripped bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := append(l.prune(key, window, now), now)
	l.hits[key] = kept
	return len(kept) >= max, len(kept) == max
}
