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

package txlog

import (
	"testing"

	"github.com/LFDT-Iguazu/iguazu-types/common"
)

func TestUpdateBalanceLogData_Copy(t *testing.T) {
	upBalData := UpdateBalanceLogData{
		InitTxHash:     common.Hash{0x11},
		CreatorAddress: common.Address{0x22},
		ProcEpoch:      6,
		Amount:         common.BigGwei,
	}

	cmp := *upBalData.Copy()
	assertEqual(t, upBalData, cmp)

	upBalDataEmpty := UpdateBalanceLogData{}
	cmpEmpty := *upBalDataEmpty.Copy()
	assertEqual(t, upBalDataEmpty, cmpEmpty)
}

func TestUpdateBalanceLogData_Marshaling(t *testing.T) {
	upBalData := &UpdateBalanceLogData{
		InitTxHash:     common.Hash{0x11},
		CreatorAddress: common.Address{0x22},
		ProcEpoch:      6,
		Amount:         common.BigGwei,
	}

	bin, err := upBalData.MarshalBinary()
	assertNoError(t, err)

	unmarshaled := &UpdateBalanceLogData{}
	err = unmarshaled.UnmarshalBinary(bin)
	assertNoError(t, err)
	assertEqual(t, upBalData, unmarshaled)
}

func TestPackUpdateBalanceLogData(t *testing.T) {
	var (
		initTxHash     = common.Hash{0x11}
		creatorAddress = common.Address{0x22}
		procEpoch      = uint64(753)
		amount         = common.BigGwei
		binLogData     = common.Hex2Bytes("f83ea01100000000000000000000000000000000000000000000000000000000000000" +
			"9422000000000000000000000000000000000000008202f1843b9aca00")
	)
	data, err := PackUpdateBalanceLogData(initTxHash, creatorAddress, procEpoch, amount)
	assertNoError(t, err)
	assertEqual(t, binLogData, data)
}

func TestUnpackUpdateBalanceLogData(t *testing.T) {
	var (
		initTxHash     = common.Hash{0x11}
		creatorAddress = common.Address{0x22}
		procEpoch      = uint64(753)
		amount         = common.BigGwei
		binLogData     = common.Hex2Bytes("f83ea01100000000000000000000000000000000000000000000000000000000000000" +
			"9422000000000000000000000000000000000000008202f1843b9aca00")
	)
	initTx, creator, proc, amt, err := UnpackUpdateBalanceLogData(binLogData)
	assertNoError(t, err)
	assertEqual(t, initTxHash, initTx)
	assertEqual(t, creatorAddress, creator)
	assertEqual(t, procEpoch, proc)
	assertEqual(t, amount, amt)
}
