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

package storage

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	blockdagtypes "github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types/era"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

// test data

var (
	testValidatorsStateAddress = common.HexToAddress("0x1234567890123456789012345678901234567890")

	testChainConfig = &ChainConfig{
		SlotsPerEpoch:          32,
		ValidatorsStateAddress: &testValidatorsStateAddress,
		ValidatorsPerSlot:      2,
		UpValidatorsPerSlot:    2,
		ForkSlotUpValsPerSlot:  0,
	}

	addr1  = common.HexToAddress("0x0000000000000000000000000000000000000001")
	addr2  = common.HexToAddress("0x0000000000000000000000000000000000000002")
	addr3  = common.HexToAddress("0x0000000000000000000000000000000000000003")
	addr4  = common.HexToAddress("0x0000000000000000000000000000000000000004")
	addr5  = common.HexToAddress("0x0000000000000000000000000000000000000005")
	addr6  = common.HexToAddress("0x0000000000000000000000000000000000000006")
	addr7  = common.HexToAddress("0x0000000000000000000000000000000000000007")
	addr8  = common.HexToAddress("0x0000000000000000000000000000000000000008")
	addr9  = common.HexToAddress("0x0000000000000000000000000000000000000009")
	addr10 = common.HexToAddress("0x000000000000000000000000000000000000000a")
	addr11 = common.HexToAddress("0x000000000000000000000000000000000000000b")
	addr12 = common.HexToAddress("0x000000000000000000000000000000000000000c")

	inputValidators = []common.Address{addr1, addr2, addr3, addr4, addr5, addr6, addr7, addr8, addr9, addr10, addr11, addr12}
)

// test helpers

func assertEqualT(t *testing.T, exp, act interface{}) {
	t.Helper()
	if !reflect.DeepEqual(exp, act) {
		t.Fatalf("\n\tExpect:\t%v\n\tGot:\t%v", exp, act)
	}
}

func assertNoErrorT(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("No error expected, but got: %s", err)
	}
}

func assertErrorT(t *testing.T, err, expErr error) {
	t.Helper()
	if err == nil {
		t.Fatalf("Expected error %v, but got nil", expErr)
	}
	if err != expErr {
		t.Fatalf("Expected error %v, but got %v", expErr, err)
	}
}

func assertNilT(t *testing.T, v interface{}) {
	t.Helper()
	if v != nil && !reflect.ValueOf(v).IsNil() {
		t.Fatalf("Expected nil, but got: %v", v)
	}
}

// newTestBlockchain returns a mockBlockchain wired to the given stateDB.
func newTestBlockchain(stateDb iface.StateDB, slotInfo *blockdagtypes.SlotInfo, eraNum uint64) *mockBlockchain {
	return &mockBlockchain{
		stateAtFn: func(root common.Hash) (iface.StateDB, error) {
			return stateDb, nil
		},
		getBlockFn: func(_ context.Context, _ common.Hash) *blockdagtypes.BlockInfo {
			return &blockdagtypes.BlockInfo{}
		},
		getSlotInfoFn: func() *blockdagtypes.SlotInfo { return slotInfo },
		getLastCCFn: func() *blockdagtypes.Checkpoint {
			return &blockdagtypes.Checkpoint{}
		},
		getEpochFn: func(_ uint64) common.Hash {
			return common.Hash{0x11}
		},
		epochToEraFn: func(_ uint64) *era.Era {
			return &era.Era{Number: eraNum}
		},
		unlockVerifiersFn: func(_ [][]common.Address) error {
			return nil
		},
	}
}

func TestConsensus_breakByValidatorsBySlotCount(t *testing.T) {
	tests := []struct {
		name              string
		validatorsPerSlot uint64
		want              [][]common.Address
	}{
		{
			name:              "5 validators per slot",
			validatorsPerSlot: 5,
			want: [][]common.Address{
				{addr1, addr2, addr3, addr4, addr5},
				{addr6, addr7, addr8, addr9, addr10},
			},
		},
		{
			name:              "2 validators per slot",
			validatorsPerSlot: 2,
			want: [][]common.Address{
				{addr1, addr2},
				{addr3, addr4},
				{addr5, addr6},
				{addr7, addr8},
				{addr9, addr10},
				{addr11, addr12},
			},
		},
		{
			name:              "1 validator per slot",
			validatorsPerSlot: 1,
			want: [][]common.Address{
				{addr1}, {addr2}, {addr3}, {addr4}, {addr5}, {addr6},
				{addr7}, {addr8}, {addr9}, {addr10}, {addr11}, {addr12},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validators := breakByValidatorsBySlotCount(inputValidators, test.validatorsPerSlot, testChainConfig.SlotsPerEpoch)
			assertEqualT(t, test.want, validators)
		})
	}
}

func TestGetValidators(t *testing.T) {
	stateDb := newMockStateDB()
	store := NewStorage(testChainConfig)
	store.SetValidatorsList(stateDb, inputValidators)

	slotInfo := &blockdagtypes.SlotInfo{
		GenesisTime:    uint64(time.Now().Unix()),
		SecondsPerSlot: 4,
		SlotsPerEpoch:  testChainConfig.SlotsPerEpoch,
	}
	bc := newTestBlockchain(stateDb, slotInfo, 0)

	for i, address := range inputValidators {
		validator := &Validator{
			Address:       address,
			Index:         uint64(i),
			ActivationEra: 0,
			ExitEra:       math.MaxUint64,
		}
		err := store.SetValidator(stateDb, validator)
		assertNoErrorT(t, err)
	}

	tests := []struct {
		name          string
		slot          uint64
		wantAddresses []common.Address
	}{
		{
			name:          "correct test",
			slot:          uint64(len(inputValidators) * int(testChainConfig.SlotsPerEpoch) / 2),
			wantAddresses: inputValidators,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			addresses, err := store.GetValidators(bc, test.slot, "Tests")
			assertNoErrorT(t, err)
			assertEqualT(t, test.wantAddresses, addresses)
		})
	}
}

func TestGetShuffledValidators(t *testing.T) {
	slot := uint64(200)
	epoch := uint64(0)

	stateDb := newMockStateDB()

	slotInfo := &blockdagtypes.SlotInfo{
		GenesisTime:    uint64(time.Now().Unix()),
		SecondsPerSlot: 4,
		SlotsPerEpoch:  testChainConfig.SlotsPerEpoch,
	}
	bc := &mockBlockchain{
		stateAtFn: func(_ common.Hash) (iface.StateDB, error) { return stateDb, nil },
		getBlockFn: func(_ context.Context, _ common.Hash) *blockdagtypes.BlockInfo {
			return &blockdagtypes.BlockInfo{}
		},
		getSlotInfoFn:     func() *blockdagtypes.SlotInfo { return slotInfo },
		getLastCCFn:       func() *blockdagtypes.Checkpoint { return &blockdagtypes.Checkpoint{} },
		getEpochFn:        func(_ uint64) common.Hash { return common.Hash{0x11} },
		epochToEraFn:      func(_ uint64) *era.Era { return &era.Era{Number: 10} },
		unlockVerifiersFn: func(_ [][]common.Address) error { return nil },
	}

	store := NewStorage(testChainConfig)
	store.SetValidatorsList(stateDb, inputValidators)

	for i, address := range inputValidators {
		validator := &Validator{
			Address:       address,
			Index:         uint64(i),
			ActivationEra: 0,
			ExitEra:       math.MaxUint64,
		}
		err := store.SetValidator(stateDb, validator)
		assertNoErrorT(t, err)
	}

	// Test case 1: Invalid filter error (too many args)
	result, err := store.GetCreatorsBySlot(bc, slot, epoch, slot)
	assertErrorT(t, err, ErrInvalidValidatorsFilter)
	assertNilT(t, result)

	// Test case 2: Validators available in cache after first shuffle
	result, err = store.GetCreatorsBySlot(bc, slot)
	assertNoErrorT(t, err)
	assertEqualT(t, []common.Address{addr5, addr4}, result)
}

func BenchmarkPrepareNextEraValidators(b *testing.B) {
	validators := make([]common.Address, 1000000)

	stateDb := newMockStateDB()
	store := NewStorage(testChainConfig)

	for i := range validators {
		addr := common.HexToAddress(fmt.Sprintf("0x%x", i))
		validators[i] = addr
		err := store.SetValidator(stateDb, &Validator{
			Address:       addr,
			Index:         uint64(i),
			ActivationEra: 0,
			ExitEra:       math.MaxUint64,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	store.SetValidatorsList(stateDb, validators)

	bc := &mockBlockchain{
		stateAtFn:     func(_ common.Hash) (iface.StateDB, error) { return stateDb, nil },
		getSlotInfoFn: func() *blockdagtypes.SlotInfo { return &blockdagtypes.SlotInfo{SlotsPerEpoch: 32} },
		epochToEraFn:  func(_ uint64) *era.Era { return &era.Era{Number: 0} },
	}

	sizes := []int{1000, 10000, 100000, 500000, 1000000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			testValidators := validators[:size]
			store.SetValidatorsList(stateDb, testValidators)

			curEra := &era.Era{
				Number:    0,
				From:      0,
				To:        0,
				Root:      common.Hash{},
				BlockHash: common.Hash{},
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				store.PrepareNextEraValidators(bc, curEra)
			}
		})
	}
}
