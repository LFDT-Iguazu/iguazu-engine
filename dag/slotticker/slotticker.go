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
	"log/slog"
	"sync"
	"time"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
)

// SlotTicker delivers the number of each slot on its channel at the moment
// the slot starts. Slot boundaries are taken from the slot info, so the ticker
// follows the slot duration fork without a restart.
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

// NewSlotTicker starts a ticker for the slots of the given slot info.
//
// Before genesis and during slot 0 the first tick is slot 0, delivered at
// genesis (immediately if genesis has passed). Later, the first tick is the
// next slot boundary.
func NewSlotTicker(si *types.SlotInfo) *SlotTicker {
	if si == nil || si.GenesisTime == 0 {
		panic("zero genesis time")
	}
	if si.SecondsPerSlot == 0 {
		panic("zero slot duration")
	}
	t := &SlotTicker{
		c:    make(chan uint64),
		done: make(chan struct{}),
	}
	go t.run(si.Copy(), realClock)
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
func firstTick(si *types.SlotInfo, now time.Time) (uint64, time.Time, error) {
	current := si.SlotByTimestamp(uint64(now.Unix()))
	if current == 0 {
		return 0, time.Unix(int64(si.GenesisTime), 0), nil
	}
	at, err := si.StartSlotTime(current + 1)
	return current + 1, at, err
}

func (t *SlotTicker) run(si *types.SlotInfo, clk clock) {
	slot, at, err := firstTick(si, clk.now())
	for err == nil {
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
		at, err = si.StartSlotTime(slot)
	}
	slog.Error("Slot ticker stopped: start slot time failed", "slot", slot, "err", err)
}
