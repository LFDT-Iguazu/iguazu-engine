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

package token

import (
	"math/big"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
)

// mockStateDB is a minimal in-memory implementation of iface.StateDB for testing.
type mockStateDB struct {
	balances  map[[20]byte]*big.Int
	nonces    map[[20]byte]uint64
	storage   map[[20]byte]map[[32]byte][32]byte
	code      map[[20]byte][]byte
	accounts  map[[20]byte]bool
	snapshots []mockSnapshot
	logs      []*iface.LogEntry
}

type mockSnapshot struct {
	balances map[[20]byte]*big.Int
	nonces   map[[20]byte]uint64
	storage  map[[20]byte]map[[32]byte][32]byte
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

func (s *mockStateDB) getBalance(addr [20]byte) *big.Int {
	if b, ok := s.balances[addr]; ok {
		return new(big.Int).Set(b)
	}
	return new(big.Int)
}

func (s *mockStateDB) GetBalance(addr [20]byte) *big.Int {
	return s.getBalance(addr)
}

func (s *mockStateDB) SetBalance(addr [20]byte, amount *big.Int) {
	s.balances[addr] = new(big.Int).Set(amount)
}

func (s *mockStateDB) AddBalance(addr [20]byte, amount *big.Int) {
	s.balances[addr] = new(big.Int).Add(s.getBalance(addr), amount)
}

func (s *mockStateDB) SubBalance(addr [20]byte, amount *big.Int) {
	s.balances[addr] = new(big.Int).Sub(s.getBalance(addr), amount)
}

func (s *mockStateDB) IsValidatorAddress(_ [20]byte) bool { return false }

func (s *mockStateDB) GetState(addr [20]byte, key [32]byte) [32]byte {
	if slots, ok := s.storage[addr]; ok {
		return slots[key]
	}
	return [32]byte{}
}

func (s *mockStateDB) SetState(addr [20]byte, key [32]byte, value [32]byte) {
	if _, ok := s.storage[addr]; !ok {
		s.storage[addr] = make(map[[32]byte][32]byte)
	}
	s.storage[addr][key] = value
}

func (s *mockStateDB) GetCode(addr [20]byte) []byte {
	return s.code[addr]
}

func (s *mockStateDB) SetCode(addr [20]byte, code []byte) {
	s.code[addr] = code
}

func (s *mockStateDB) GetNonce(addr [20]byte) uint64 {
	return s.nonces[addr]
}

func (s *mockStateDB) SetNonce(addr [20]byte, nonce uint64) {
	s.nonces[addr] = nonce
}

func (s *mockStateDB) Exist(addr [20]byte) bool {
	return s.accounts[addr]
}

func (s *mockStateDB) CreateAccount(addr [20]byte) {
	s.accounts[addr] = true
}

func (s *mockStateDB) Suicide(addr [20]byte) bool {
	if !s.accounts[addr] {
		return false
	}
	delete(s.accounts, addr)
	delete(s.balances, addr)
	return true
}

func (s *mockStateDB) AddLog(log *iface.LogEntry) {
	s.logs = append(s.logs, log)
}

func (s *mockStateDB) Snapshot() int {
	snap := mockSnapshot{
		balances: make(map[[20]byte]*big.Int, len(s.balances)),
		nonces:   make(map[[20]byte]uint64, len(s.nonces)),
		storage:  make(map[[20]byte]map[[32]byte][32]byte, len(s.storage)),
		accounts: make(map[[20]byte]bool, len(s.accounts)),
	}
	for k, v := range s.balances {
		snap.balances[k] = new(big.Int).Set(v)
	}
	for k, v := range s.nonces {
		snap.nonces[k] = v
	}
	for addr, slots := range s.storage {
		snapSlots := make(map[[32]byte][32]byte, len(slots))
		for k, v := range slots {
			snapSlots[k] = v
		}
		snap.storage[addr] = snapSlots
	}
	for k, v := range s.accounts {
		snap.accounts[k] = v
	}
	id := len(s.snapshots)
	s.snapshots = append(s.snapshots, snap)
	return id
}

func (s *mockStateDB) RevertToSnapshot(id int) {
	if id >= len(s.snapshots) {
		return
	}
	snap := s.snapshots[id]
	s.balances = make(map[[20]byte]*big.Int, len(snap.balances))
	for k, v := range snap.balances {
		s.balances[k] = new(big.Int).Set(v)
	}
	s.nonces = make(map[[20]byte]uint64, len(snap.nonces))
	for k, v := range snap.nonces {
		s.nonces[k] = v
	}
	s.storage = make(map[[20]byte]map[[32]byte][32]byte, len(snap.storage))
	for addr, slots := range snap.storage {
		snapSlots := make(map[[32]byte][32]byte, len(slots))
		for k, v := range slots {
			snapSlots[k] = v
		}
		s.storage[addr] = snapSlots
	}
	s.accounts = make(map[[20]byte]bool, len(snap.accounts))
	for k, v := range snap.accounts {
		s.accounts[k] = v
	}
	s.snapshots = s.snapshots[:id]
}

func (s *mockStateDB) ForEachStorage(addr [20]byte, cb func(key, value [32]byte) bool) error {
	for k, v := range s.storage[addr] {
		if !cb(k, v) {
			break
		}
	}
	return nil
}
