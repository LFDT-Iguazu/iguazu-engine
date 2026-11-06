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

package dag

import (
	"testing"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
)

const testGenesisTime = uint64(1_700_000_000)

// stubChain implements the part of the block chain used by HandleSyncSlotInfo.
type stubChain struct {
	blockChain
	si     *types.SlotInfo
	cfg    *iface.ChainConfig
	stored *types.SlotInfo
}

func (c *stubChain) GetSlotInfo() *types.SlotInfo { return c.si }
func (c *stubChain) Config() *iface.ChainConfig   { return c.cfg }
func (c *stubChain) SetSlotInfo(si *types.SlotInfo) error {
	c.stored = si
	return nil
}

// stubBackend implements the part of the backend used by HandleSyncSlotInfo.
type stubBackend struct {
	Backend
	devMode bool
}

func (b stubBackend) IsDevMode() bool { return b.devMode }

// TestHandleSyncSlotInfo checks that slot settings from the coordinating network are
// matched against the local ones and completed with the local slot duration fork
// settings, which the coordinator never reports. The fork slot is far in the past,
// so the slot duration in effect is the forked one.
func TestHandleSyncSlotInfo(t *testing.T) {
	cfg := &iface.ChainConfig{ForkSlotUpSecondsPerSlot: 100, UpSecondsPerSlot: 3}
	local := &types.SlotInfo{
		GenesisTime:              testGenesisTime,
		SecondsPerSlot:           6,
		SlotsPerEpoch:            32,
		ForkSlotUpSecondsPerSlot: 100,
		UpSecondsPerSlot:         3,
	}
	coord := func(secondsPerSlot uint64) types.SlotInfo {
		return types.SlotInfo{GenesisTime: testGenesisTime, SecondsPerSlot: secondsPerSlot, SlotsPerEpoch: 32}
	}

	tests := []struct {
		name       string
		local      *types.SlotInfo
		coord      types.SlotInfo
		devMode    bool
		wantOk     bool
		wantStored *types.SlotInfo
	}{
		{name: "base slot duration matches", local: local, coord: coord(6), wantOk: true},
		{name: "forked slot duration matches", local: local, coord: coord(3), wantOk: true},
		{name: "unrelated slot duration is rejected", local: local, coord: coord(4), wantOk: false},
		{name: "initial slot info gets the local fork settings", coord: coord(6), wantOk: true, wantStored: local},
		{
			name:    "dev mode override keeps the local fork settings",
			local:   local,
			coord:   coord(4),
			devMode: true,
			wantOk:  true,
			wantStored: &types.SlotInfo{
				GenesisTime:              testGenesisTime,
				SecondsPerSlot:           4,
				SlotsPerEpoch:            32,
				ForkSlotUpSecondsPerSlot: 100,
				UpSecondsPerSlot:         3,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := &stubChain{si: tt.local, cfg: cfg}
			d := Dag{eth: stubBackend{devMode: tt.devMode}, bc: bc}

			ok, err := d.HandleSyncSlotInfo(tt.coord)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tt.wantOk {
				t.Fatalf("HandleSyncSlotInfo() = %v, want %v", ok, tt.wantOk)
			}
			switch {
			case tt.wantStored == nil && bc.stored != nil:
				t.Fatalf("slot info must not be stored, got %+v", bc.stored)
			case tt.wantStored != nil && (bc.stored == nil || *bc.stored != *tt.wantStored):
				t.Fatalf("stored = %+v, want %+v", bc.stored, tt.wantStored)
			}
		})
	}
}
