// Copyright 2026 Digital Clever Solution LLC
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

package storage

import "github.com/LFDT-Iguazu/iguazu-types/common"

// ChainConfig holds the minimal chain configuration needed by the storage package.
type ChainConfig struct {
	SlotsPerEpoch          uint64
	ValidatorsStateAddress *common.Address
	ValidatorsPerSlot      uint64
	UpValidatorsPerSlot    uint64
	ForkSlotUpValsPerSlot  uint64
}

// GetValidatorsPerSlot returns the validators-per-slot count,
// switching to UpValidatorsPerSlot after ForkSlotUpValsPerSlot.
func (c *ChainConfig) GetValidatorsPerSlot(slot uint64) uint64 {
	if slot >= c.ForkSlotUpValsPerSlot && c.UpValidatorsPerSlot > 0 {
		return c.UpValidatorsPerSlot
	}
	return c.ValidatorsPerSlot
}
