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
	"math/big"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	blockdagtypes "github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types/era"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

// mockStateDB is a minimal in-memory implementation of iface.StateDB for testing.
type mockStateDB struct {
	balances map[[20]byte]*big.Int
	nonces   map[[20]byte]uint64
	storage  map[[20]byte]map[[32]byte][32]byte
	code     map[[20]byte][]byte
	accounts map[[20]byte]bool
}

func newMockStateDB() *mockStateDB {
	return &mockStateDB{
		balances: make(map[[20]byte]*big.Int),
		nonces:   make(map[[20]byte]uint64),
		storage:  make(map[[20]byte]map[[32]byte][32]byte),
		code:     make(map[[20]byte][]byte),
		accounts: make(map[[20]byte]bool),
	}
}

func (m *mockStateDB) GetBalance(addr [20]byte) *big.Int {
	if b, ok := m.balances[addr]; ok {
		return new(big.Int).Set(b)
	}
	return new(big.Int)
}
func (m *mockStateDB) SetBalance(addr [20]byte, amount *big.Int) {
	m.balances[addr] = new(big.Int).Set(amount)
}
func (m *mockStateDB) AddBalance(addr [20]byte, amount *big.Int) {
	cur := new(big.Int)
	if b, ok := m.balances[addr]; ok {
		cur.Set(b)
	}
	m.balances[addr] = cur.Add(cur, amount)
}
func (m *mockStateDB) SubBalance(addr [20]byte, amount *big.Int) {
	cur := new(big.Int)
	if b, ok := m.balances[addr]; ok {
		cur.Set(b)
	}
	m.balances[addr] = cur.Sub(cur, amount)
}
func (m *mockStateDB) IsValidatorAddress(_ [20]byte) bool { return false }
func (m *mockStateDB) GetState(addr [20]byte, key [32]byte) [32]byte {
	if slots, ok := m.storage[addr]; ok {
		return slots[key]
	}
	return [32]byte{}
}
func (m *mockStateDB) SetState(addr [20]byte, key [32]byte, value [32]byte) {
	if _, ok := m.storage[addr]; !ok {
		m.storage[addr] = make(map[[32]byte][32]byte)
	}
	m.storage[addr][key] = value
}
func (m *mockStateDB) GetCode(addr [20]byte) []byte         { return m.code[addr] }
func (m *mockStateDB) SetCode(addr [20]byte, code []byte)   { m.code[addr] = code }
func (m *mockStateDB) GetNonce(addr [20]byte) uint64        { return m.nonces[addr] }
func (m *mockStateDB) SetNonce(addr [20]byte, nonce uint64) { m.nonces[addr] = nonce }
func (m *mockStateDB) Exist(addr [20]byte) bool             { return m.accounts[addr] }
func (m *mockStateDB) CreateAccount(addr [20]byte)          { m.accounts[addr] = true }
func (m *mockStateDB) Suicide(addr [20]byte) bool {
	if !m.accounts[addr] {
		return false
	}
	delete(m.accounts, addr)
	delete(m.balances, addr)
	return true
}
func (m *mockStateDB) AddLog(_ *iface.LogEntry) {}
func (m *mockStateDB) Snapshot() int            { return 0 }
func (m *mockStateDB) RevertToSnapshot(_ int)   {}
func (m *mockStateDB) ForEachStorage(_ [20]byte, _ func(key, value [32]byte) bool) error {
	return nil
}

// mockBlockchain is a simple test double for the blockchain interface.
type mockBlockchain struct {
	stateAtFn         func(root common.Hash) (iface.StateDB, error)
	getBlockFn        func(ctx context.Context, hash common.Hash) *blockdagtypes.BlockInfo
	getSlotInfoFn     func() *blockdagtypes.SlotInfo
	getLastCCFn       func() *blockdagtypes.Checkpoint
	getEpochFn        func(epoch uint64) common.Hash
	epochToEraFn      func(uint64) *era.Era
	unlockVerifiersFn func(accounts [][]common.Address) error
}

func (m *mockBlockchain) StateAt(root common.Hash) (iface.StateDB, error) {
	return m.stateAtFn(root)
}
func (m *mockBlockchain) GetBlock(ctx context.Context, hash common.Hash) *blockdagtypes.BlockInfo {
	return m.getBlockFn(ctx, hash)
}
func (m *mockBlockchain) GetSlotInfo() *blockdagtypes.SlotInfo {
	return m.getSlotInfoFn()
}
func (m *mockBlockchain) GetLastCoordinatedCheckpoint() *blockdagtypes.Checkpoint {
	return m.getLastCCFn()
}
func (m *mockBlockchain) GetEpoch(epoch uint64) common.Hash {
	return m.getEpochFn(epoch)
}
func (m *mockBlockchain) EpochToEra(epoch uint64) *era.Era {
	return m.epochToEraFn(epoch)
}
func (m *mockBlockchain) UnlockVerifiers(accounts [][]common.Address) error {
	return m.unlockVerifiersFn(accounts)
}
