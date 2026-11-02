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
	"crypto/ecdsa"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	blockdagtypes "github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

// --- mocks ---

type mockInternalKeyStore struct {
	accounts  []iface.Account
	wallets   []iface.Wallet
	unlocked  map[common.Address]bool
	passwords map[common.Address]string
}

func newMockInternalKeyStore(addrs ...common.Address) *mockInternalKeyStore {
	accounts := make([]iface.Account, len(addrs))
	for i, a := range addrs {
		accounts[i] = iface.Account{Address: a}
	}
	return &mockInternalKeyStore{
		accounts:  accounts,
		unlocked:  make(map[common.Address]bool),
		passwords: make(map[common.Address]string),
	}
}

func (m *mockInternalKeyStore) Accounts() []iface.Account { return m.accounts }
func (m *mockInternalKeyStore) Wallets() []iface.Wallet   { return m.wallets }
func (m *mockInternalKeyStore) ScanAccounts() error       { return nil }
func (m *mockInternalKeyStore) IsUnlocked(addr common.Address) bool {
	return m.unlocked[addr]
}
func (m *mockInternalKeyStore) GetKey(addr common.Address) (*ecdsa.PrivateKey, error) {
	return nil, errors.New("not implemented")
}
func (m *mockInternalKeyStore) SignTx(a iface.Account, tx blockdagtypes.Transaction, _ *big.Int) (blockdagtypes.Transaction, error) {
	// echo back with a marker byte prepended to simulate signing
	signed := make([]byte, len(tx.Raw)+1)
	signed[0] = 0xff
	copy(signed[1:], tx.Raw)
	return blockdagtypes.Transaction{Raw: signed}, nil
}
func (m *mockInternalKeyStore) Unlock(account iface.Account, passphrase string) error {
	if exp, ok := m.passwords[account.Address]; ok && exp != passphrase {
		return errors.New("wrong password")
	}
	m.unlocked[account.Address] = true
	return nil
}

// mockWallet implements iface.Wallet.
type mockWallet struct {
	addr common.Address
}

func (w *mockWallet) Contains(acc iface.Account) bool { return acc.Address == w.addr }

// mockKeystoreConfig implements iface.KeystoreConfig.
type mockKeystoreConfig struct {
	keyDir       string
	passwordFile string
}

func (c *mockKeystoreConfig) KeyDir() (string, error)            { return c.keyDir, nil }
func (c *mockKeystoreConfig) PasswordsFilePath() (string, error) { return c.passwordFile, nil }

// mockPrompter implements iface.PasswordPrompter.
type mockPrompter struct{ password string }

func (p *mockPrompter) PromptPassword(_ string) (string, error) { return p.password, nil }

// --- helpers ---

func newTestKeystore(t *testing.T, addrs ...common.Address) (VerifiersKeystore, *mockInternalKeyStore) {
	t.Helper()
	ks := newMockInternalKeyStore(addrs...)
	cfg := &mockKeystoreConfig{}
	vk := NewKeystore(cfg, ks, &mockPrompter{})
	return vk, ks
}

// writePasswordFile creates a temp file with the given passwords (one per line).
func writePasswordFile(t *testing.T, passwords ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "passwords.txt")
	content := ""
	for _, p := range passwords {
		content += p + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- tests ---

func TestVerifiersAddresses(t *testing.T) {
	vk, _ := newTestKeystore(t, addr1, addr2, addr3)
	got := vk.VerifiersAddresses()
	assertEqualT(t, []common.Address{addr1, addr2, addr3}, got)
}

func TestIsVerifierUnlocked(t *testing.T) {
	vk, ks := newTestKeystore(t, addr1)
	assertEqualT(t, false, vk.IsVerifierUnlocked(addr1))
	ks.unlocked[addr1] = true
	assertEqualT(t, true, vk.IsVerifierUnlocked(addr1))
}

func TestFindWallet(t *testing.T) {
	vk, ks := newTestKeystore(t, addr1, addr2)
	ks.wallets = []iface.Wallet{&mockWallet{addr: addr1}, &mockWallet{addr: addr2}}

	w, err := vk.FindWallet(iface.Account{Address: addr1})
	assertNoErrorT(t, err)
	if w == nil {
		t.Fatal("expected wallet, got nil")
	}

	_, err = vk.FindWallet(iface.Account{Address: addr3})
	if err == nil {
		t.Fatal("expected error for unknown account")
	}
}

func TestSignTx(t *testing.T) {
	vk, _ := newTestKeystore(t, addr1)
	raw := []byte{0x01, 0x02, 0x03}
	tx := blockdagtypes.Transaction{Raw: raw}

	signed, err := vk.SignTx(iface.Account{Address: addr1}, tx, big.NewInt(1))
	assertNoErrorT(t, err)
	if len(signed.Raw) == 0 {
		t.Fatal("expected signed transaction bytes")
	}
	// mock prepends 0xff
	assertEqualT(t, byte(0xff), signed.Raw[0])
	assertEqualT(t, raw, signed.Raw[1:])
}

func TestUnlockAccounts(t *testing.T) {
	passwordFile := writePasswordFile(t, "secret", "secret")
	ks := newMockInternalKeyStore(addr1, addr2)
	ks.passwords[addr1] = "secret"
	ks.passwords[addr2] = "secret"

	cfg := &mockKeystoreConfig{passwordFile: passwordFile}
	vk := NewKeystore(cfg, ks, &mockPrompter{password: "secret"})

	err := vk.UnlockAccounts([][]common.Address{{addr1, addr2}})
	assertNoErrorT(t, err)
	assertEqualT(t, true, ks.unlocked[addr1])
	assertEqualT(t, true, ks.unlocked[addr2])
}

func TestUnlockAllAccounts(t *testing.T) {
	passwordFile := writePasswordFile(t, "pass", "pass")
	ks := newMockInternalKeyStore(addr1, addr2)
	ks.passwords[addr1] = "pass"
	ks.passwords[addr2] = "pass"

	cfg := &mockKeystoreConfig{passwordFile: passwordFile}
	vk := NewKeystore(cfg, ks, &mockPrompter{password: "pass"})

	vk.UnlockAllAccounts()
	assertEqualT(t, true, ks.unlocked[addr1])
	assertEqualT(t, true, ks.unlocked[addr2])
}

func TestVerifiersWallets(t *testing.T) {
	vk, ks := newTestKeystore(t, addr1)
	ks.wallets = []iface.Wallet{&mockWallet{addr: addr1}}

	wallets := vk.VerifiersWallets()
	assertEqualT(t, 1, len(wallets))
}
