// Copyright 2026 Digital Clever Solution LLC
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package slotticker

import (
	"testing"
	"time"
)

func TestFirstTick(t *testing.T) {
	genesis := time.Unix(1_700_000_000, 0)
	const d = 6 * time.Second
	cases := []struct {
		name     string
		now      time.Time
		wantSlot uint64
		wantAt   time.Time
	}{
		{"before genesis", genesis.Add(-time.Hour), 0, genesis},
		{"at genesis", genesis, 0, genesis},
		{"inside slot 0", genesis.Add(5 * time.Second), 0, genesis},
		{"start of slot 1", genesis.Add(d), 2, genesis.Add(2 * d)},
		{"inside slot 1", genesis.Add(d + time.Second), 2, genesis.Add(2 * d)},
		{"inside slot 41", genesis.Add(41*d + 3*time.Second), 42, genesis.Add(42 * d)},
	}
	for _, c := range cases {
		slot, at := firstTick(genesis, c.now, d)
		if slot != c.wantSlot || !at.Equal(c.wantAt) {
			t.Errorf("%s: got slot %d at %v, want slot %d at %v", c.name, slot, at, c.wantSlot, c.wantAt)
		}
	}
}

// fakeClock fires every timer immediately and advances time to its deadline.
type fakeClock struct {
	now   time.Time
	waits chan time.Duration
}

func (f *fakeClock) clock() clock {
	return clock{
		now: func() time.Time { return f.now },
		after: func(d time.Duration) <-chan time.Time {
			f.waits <- d
			f.now = f.now.Add(d)
			c := make(chan time.Time, 1)
			c <- f.now
			return c
		},
	}
}

func TestTicks(t *testing.T) {
	genesis := time.Unix(1_700_000_000, 0)
	const d = 5 * time.Second
	f := &fakeClock{now: genesis.Add(12 * time.Second), waits: make(chan time.Duration, 16)}
	ticker := &SlotTicker{c: make(chan uint64), done: make(chan struct{})}
	go ticker.run(genesis, d, f.clock())

	// Slot 3 starts 3s from now, then a tick every slot.
	for i, want := range []uint64{3, 4, 5} {
		if got := <-ticker.C(); got != want {
			t.Fatalf("tick %d = slot %d, want %d", i, got, want)
		}
	}
	if first := <-f.waits; first != 3*time.Second {
		t.Errorf("first wait = %v, want 3s", first)
	}
	if next := <-f.waits; next != d {
		t.Errorf("next wait = %v, want %v", next, d)
	}

	ticker.Done()
	ticker.Done() // must not block or panic
	select {
	case slot, ok := <-ticker.C():
		if ok {
			// a tick that was already pending may still be delivered once
			t.Logf("pending tick %d delivered after Done", slot)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestNewSlotTickerPanics(t *testing.T) {
	for name, f := range map[string]func(){
		"zero genesis":  func() { NewSlotTicker(time.Time{}, 6) },
		"zero duration": func() { NewSlotTicker(time.Now(), 0) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: expected a panic", name)
				}
			}()
			f()
		}()
	}
}
