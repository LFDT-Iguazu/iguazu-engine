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
	"bytes"
	"errors"
	"math/big"
	"math/rand"
	"testing"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
)

// mockStateDB is a minimal in-memory StateDB for storage tests.
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

func (s *mockStateDB) getBalance(addr [20]byte) *big.Int {
	if b, ok := s.balances[addr]; ok {
		return new(big.Int).Set(b)
	}
	return new(big.Int)
}
func (s *mockStateDB) GetBalance(addr [20]byte) *big.Int { return s.getBalance(addr) }
func (s *mockStateDB) SetBalance(addr [20]byte, a *big.Int) {
	s.balances[addr] = new(big.Int).Set(a)
}
func (s *mockStateDB) AddBalance(addr [20]byte, a *big.Int) {
	s.balances[addr] = new(big.Int).Add(s.getBalance(addr), a)
}
func (s *mockStateDB) SubBalance(addr [20]byte, a *big.Int) {
	s.balances[addr] = new(big.Int).Sub(s.getBalance(addr), a)
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
func (s *mockStateDB) GetCode(addr [20]byte) []byte       { return s.code[addr] }
func (s *mockStateDB) SetCode(addr [20]byte, code []byte) { s.code[addr] = code }
func (s *mockStateDB) GetNonce(addr [20]byte) uint64      { return s.nonces[addr] }
func (s *mockStateDB) SetNonce(addr [20]byte, n uint64)   { s.nonces[addr] = n }
func (s *mockStateDB) Exist(addr [20]byte) bool           { return s.accounts[addr] }
func (s *mockStateDB) CreateAccount(addr [20]byte)        { s.accounts[addr] = true }
func (s *mockStateDB) Suicide(addr [20]byte) bool {
	if !s.accounts[addr] {
		return false
	}
	delete(s.accounts, addr)
	delete(s.balances, addr)
	return true
}
func (s *mockStateDB) AddLog(*iface.LogEntry)  {}
func (s *mockStateDB) Snapshot() int           { return 0 }
func (s *mockStateDB) RevertToSnapshot(id int) {}
func (s *mockStateDB) ForEachStorage(addr [20]byte, cb func(key, value [32]byte) bool) error {
	for k, v := range s.storage[addr] {
		if !cb(k, v) {
			break
		}
	}
	return nil
}

// --- test helpers ---

func randomData(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func randomInt(min, max int) int {
	return rand.Intn(max-min+1) + min
}

func compareBytes(t *testing.T, a, b []byte) {
	t.Helper()
	if !bytes.Equal(a, b) {
		t.Fatalf("values do not match:\n want: %+v\nhave: %+v", b, a)
	}
}

func checkError(err error, errs []error) bool {
	if err == nil && len(errs) == 0 {
		return true
	}
	for _, e := range errs {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
