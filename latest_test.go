package main

import (
	"sync"
	"testing"
	"time"
)

// On, then off before the slow "on" finishes: the last applied value must be off.
func TestLatestWins(t *testing.T) {
	var l latestWins[bool]
	var mu sync.Mutex
	var applied []bool
	var wg sync.WaitGroup
	apply := func(on bool) {
		if on {
			time.Sleep(50 * time.Millisecond) // a slow Enable
		}
		mu.Lock()
		applied = append(applied, on)
		mu.Unlock()
		wg.Done()
	}
	wg.Add(2)
	l.set(true, apply)
	l.set(false, apply)
	wg.Wait()
	if last := applied[len(applied)-1]; last {
		t.Errorf("applied %v: the last change asked for was off", applied)
	}
}
