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

// Package slotticker provides a ticker that fires at the start of every slot.
package slotticker

import (
	"sync"
	"time"
)

// SlotTicker delivers the number of each slot on its channel at the moment
// the slot starts. Slot boundaries are multiples of the slot duration counted
// from the genesis time.
type SlotTicker struct {
	c        chan uint64
	done     chan struct{}
	doneOnce sync.Once
}

// clock abstracts time for tests.
type clock struct {
	now   func() time.Time
	after func(time.Duration) <-chan time.Time
}

var realClock = clock{now: time.Now, after: time.After}

// NewSlotTicker starts a ticker for slots of secondsPerSlot seconds counted
// from genesisTime.
//
// Before genesis and during slot 0 the first tick is slot 0, delivered at
// genesis (immediately if genesis has passed). Later, the first tick is the
// next slot boundary.
func NewSlotTicker(genesisTime time.Time, secondsPerSlot uint64) *SlotTicker {
	if genesisTime.IsZero() {
		panic("zero genesis time")
	}
	if secondsPerSlot == 0 {
		panic("zero slot duration")
	}
	t := &SlotTicker{
		c:    make(chan uint64),
		done: make(chan struct{}),
	}
	go t.run(genesisTime, time.Duration(secondsPerSlot)*time.Second, realClock)
	return t
}

// C returns the channel on which slot numbers are delivered.
func (t *SlotTicker) C() <-chan uint64 {
	return t.c
}

// Done stops the ticker. It may be called more than once.
func (t *SlotTicker) Done() {
	t.doneOnce.Do(func() { close(t.done) })
}

// firstTick returns the first slot to deliver and when to deliver it.
func firstTick(genesis, now time.Time, slotDuration time.Duration) (uint64, time.Time) {
	elapsed := now.Sub(genesis)
	if elapsed < slotDuration {
		return 0, genesis
	}
	slot := uint64(elapsed/slotDuration) + 1
	return slot, genesis.Add(time.Duration(slot) * slotDuration)
}

func (t *SlotTicker) run(genesis time.Time, slotDuration time.Duration, clk clock) {
	slot, at := firstTick(genesis, clk.now(), slotDuration)
	for {
		select {
		case <-clk.after(at.Sub(clk.now())):
		case <-t.done:
			return
		}
		select {
		case t.c <- slot:
		case <-t.done:
			return
		}
		slot++
		at = at.Add(slotDuration)
	}
}
