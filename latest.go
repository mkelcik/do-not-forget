package main

import "sync"

// latestWins applies changes one at a time, each with the most recent value asked for. A slow
// change can't finish after a newer one and leave the older value in place (start at login
// switched on and quickly off must end off).
type latestWins[T any] struct {
	run  sync.Mutex // held while apply runs
	mu   sync.Mutex // guards want
	want T
}

// set records v and applies it in the background.
func (l *latestWins[T]) set(v T, apply func(T)) {
	l.mu.Lock()
	l.want = v
	l.mu.Unlock()
	go func() {
		l.run.Lock()
		defer l.run.Unlock()
		l.mu.Lock()
		v := l.want
		l.mu.Unlock()
		apply(v)
	}()
}
