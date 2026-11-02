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

package txlog

import (
	"testing"

	"github.com/LFDT-Iguazu/iguazu-types/common"
)

func TestDeactivateLogData_Copy(t *testing.T) {
	upBalData := DeactivateLogData{
		InitTxHash:     common.Hash{0x11},
		CreatorAddress: common.Address{0x22},
		ProcEpoch:      6,
		ValidatorIndex: 456,
	}

	cmp := *upBalData.Copy()
	assertEqual(t, upBalData, cmp)

	upBalDataEmpty := DeactivateLogData{}
	cmpEmpty := *upBalDataEmpty.Copy()
	assertEqual(t, upBalDataEmpty, cmpEmpty)
}

func TestDeactivateLogData_Marshaling(t *testing.T) {
	upBalData := &DeactivateLogData{
		InitTxHash:     common.Hash{0x11},
		CreatorAddress: common.Address{0x22},
		ProcEpoch:      6,
		ValidatorIndex: 456,
	}

	bin, err := upBalData.MarshalBinary()
	assertNoError(t, err)

	unmarshaled := &DeactivateLogData{}
	err = unmarshaled.UnmarshalBinary(bin)
	assertNoError(t, err)
	assertEqual(t, upBalData, unmarshaled)
}

func TestPackDeactivateLogData(t *testing.T) {
	var (
		initTxHash     = common.Hash{0x11}
		creatorAddress = common.Address{0x22}
		procEpoch      = uint64(753)
		validatorIndex = uint64(456)
		binLogData     = common.Hex2Bytes("f83ca01100000000000000000000000000000000000000000000000000000000000000" +
			"9422000000000000000000000000000000000000008202f18201c8")
	)
	data, err := PackDeactivateLogData(initTxHash, creatorAddress, procEpoch, validatorIndex)
	assertNoError(t, err)
	assertEqual(t, binLogData, data)
}

func TestUnpackDeactivateLogData(t *testing.T) {
	var (
		initTxHash     = common.Hash{0x11}
		creatorAddress = common.Address{0x22}
		procEpoch      = uint64(753)
		validatorIndex = uint64(456)
		binLogData     = common.Hex2Bytes("f83ca01100000000000000000000000000000000000000000000000000000000000000" +
			"9422000000000000000000000000000000000000008202f18201c8")
	)
	initTx, creator, proc, vix, err := UnpackDeactivateLogData(binLogData)
	assertNoError(t, err)
	assertEqual(t, initTxHash, initTx)
	assertEqual(t, creatorAddress, creator)
	assertEqual(t, procEpoch, proc)
	assertEqual(t, validatorIndex, vix)
}
