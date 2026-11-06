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

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
)

const testGenesis = uint64(1_700_000_000)

// plainInfo returns slot info without the slot duration fork.
func plainInfo(secondsPerSlot uint64) *types.SlotInfo {
	return &types.SlotInfo{GenesisTime: testGenesis, SecondsPerSlot: secondsPerSlot, SlotsPerEpoch: 32}
}

// forkInfo returns slot info with slots of 6 seconds which last 3 seconds from forkSlot on.
func forkInfo(forkSlot uint64) *types.SlotInfo {
	si := plainInfo(6)
	si.ForkSlotUpSecondsPerSlot = forkSlot
	si.UpSecondsPerSlot = 3
	return si
}

func TestFirstTick(t *testing.T) {
	genesis := time.Unix(int64(testGenesis), 0)
	const d = 6 * time.Second
	cases := []struct {
		name     string
		si       *types.SlotInfo
		now      time.Time
		wantSlot uint64
		wantAt   time.Time
	}{
		{"before genesis", plainInfo(6), genesis.Add(-time.Hour), 0, genesis},
		{"at genesis", plainInfo(6), genesis, 0, genesis},
		{"inside slot 0", plainInfo(6), genesis.Add(5 * time.Second), 0, genesis},
		{"start of slot 1", plainInfo(6), genesis.Add(d), 2, genesis.Add(2 * d)},
		{"inside slot 1", plainInfo(6), genesis.Add(d + time.Second), 2, genesis.Add(2 * d)},
		{"inside slot 41", plainInfo(6), genesis.Add(41*d + 3*time.Second), 42, genesis.Add(42 * d)},
		{"last slot before fork", forkInfo(10), genesis.Add(9 * d), 10, genesis.Add(10 * d)},
		{"fork slot", forkInfo(10), genesis.Add(10 * d), 11, genesis.Add(10*d + 3*time.Second)},
		{"after fork", forkInfo(10), genesis.Add(10*d + 7*time.Second), 13, genesis.Add(10*d + 9*time.Second)},
		{"fork at genesis, inside slot 0", forkInfo(0), genesis.Add(2 * time.Second), 0, genesis},
		{"fork at genesis, inside slot 1", forkInfo(0), genesis.Add(4 * time.Second), 2, genesis.Add(6 * time.Second)},
	}
	for _, c := range cases {
		slot, at, err := firstTick(c.si, c.now)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
			continue
		}
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
	genesis := time.Unix(int64(testGenesis), 0)
	const d = 5 * time.Second
	f := &fakeClock{now: genesis.Add(12 * time.Second), waits: make(chan time.Duration, 16)}
	ticker := &SlotTicker{c: make(chan uint64), done: make(chan struct{})}
	go ticker.run(plainInfo(5), f.clock())

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

// TestTicksAcrossFork checks that tick times follow the slot duration change.
func TestTicksAcrossFork(t *testing.T) {
	genesis := time.Unix(int64(testGenesis), 0)
	f := &fakeClock{now: genesis.Add(7 * time.Second), waits: make(chan time.Duration, 16)}
	ticker := &SlotTicker{c: make(chan uint64), done: make(chan struct{})}
	go ticker.run(forkInfo(3), f.clock())
	defer ticker.Done()

	for i, want := range []uint64{2, 3, 4, 5} {
		if got := <-ticker.C(); got != want {
			t.Fatalf("tick %d = slot %d, want %d", i, got, want)
		}
	}
	// Slot 2 starts in 5s, slot 3 (the fork slot) a full 6s later, the rest every 3s.
	for i, want := range []time.Duration{5 * time.Second, 6 * time.Second, 3 * time.Second, 3 * time.Second} {
		if got := <-f.waits; got != want {
			t.Errorf("wait %d = %v, want %v", i, got, want)
		}
	}
}

func TestNewSlotTickerPanics(t *testing.T) {
	for name, f := range map[string]func(){
		"nil slot info": func() { NewSlotTicker(nil) },
		"zero genesis":  func() { NewSlotTicker(&types.SlotInfo{SecondsPerSlot: 6}) },
		"zero duration": func() { NewSlotTicker(&types.SlotInfo{GenesisTime: testGenesis}) },
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
