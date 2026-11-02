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
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	blockdagtypes "github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

// VerifiersKeystore manages validator signing keys.
type VerifiersKeystore interface {
	UnlockAccounts(accounts [][]common.Address) error
	IsVerifierUnlocked(address common.Address) bool
	UnlockAllAccounts()
	GetKey(verifier common.Address) (*ecdsa.PrivateKey, error)
	SignTx(a iface.Account, tx blockdagtypes.Transaction, chainID *big.Int) (blockdagtypes.Transaction, error)
	VerifiersAddresses() []common.Address
	VerifiersWallets() []iface.Wallet
	FindWallet(account iface.Account) (iface.Wallet, error)
}

type verifiersKeystore struct {
	keystore iface.InternalKeyStore
	cfg      iface.KeystoreConfig
	prompter iface.PasswordPrompter
}

// NewKeystore creates a VerifiersKeystore using the provided keystore implementation,
// config, and password prompter (dependency-injected by the gwat layer).
func NewKeystore(cfg iface.KeystoreConfig, ks iface.InternalKeyStore, prompter iface.PasswordPrompter) VerifiersKeystore {
	vk := &verifiersKeystore{
		keystore: ks,
		cfg:      cfg,
		prompter: prompter,
	}
	if err := vk.initVerifiers(); err != nil {
		slog.Error("Failed to initialize verifiers", "err", err)
	}
	return vk
}

// UnlockAccounts unlocks the specified accounts.
func (k *verifiersKeystore) UnlockAccounts(accounts [][]common.Address) error {
	passwords, err := k.getPasswords()
	if err != nil {
		return err
	}
	keystoreAccounts := k.keystore.Accounts()

	for _, slotAccount := range accounts {
		for _, account := range slotAccount {
			position := findAccountPosition(keystoreAccounts, account.String())
			if position < 0 {
				continue
			}
			err = k.unlockAccount(account.String(), position, passwords)
			if err != nil {
				slog.Error("unlock account failed", "error", err, "account", account.String())
			}
		}
	}
	return nil
}

func (k *verifiersKeystore) UnlockAllAccounts() {
	passwords, err := k.getPasswords()
	if err != nil {
		return
	}
	keystoreAccounts := k.keystore.Accounts()

	for _, account := range keystoreAccounts {
		position := findAccountPosition(keystoreAccounts, account.Address.String())
		if position < 0 {
			continue
		}
		err = k.unlockAccount(account.Address.String(), position, passwords)
		if err != nil {
			slog.Error("unlock account failed", "error", err, "account", account.Address.String())
		}
	}
}

func (k *verifiersKeystore) VerifiersAddresses() []common.Address {
	verifiers := k.keystore.Accounts()
	addresses := make([]common.Address, 0, len(verifiers))
	for _, account := range verifiers {
		addresses = append(addresses, account.Address)
	}
	return addresses
}

func (k *verifiersKeystore) VerifiersWallets() []iface.Wallet {
	return k.keystore.Wallets()
}

func (k *verifiersKeystore) FindWallet(account iface.Account) (iface.Wallet, error) {
	for _, wallet := range k.keystore.Wallets() {
		if wallet.Contains(account) {
			return wallet, nil
		}
	}
	return nil, errors.New("wallet not found")
}

func (k *verifiersKeystore) IsVerifierUnlocked(verifierAddress common.Address) bool {
	return k.keystore.IsUnlocked(verifierAddress)
}

func (k *verifiersKeystore) GetKey(verifier common.Address) (*ecdsa.PrivateKey, error) {
	return k.keystore.GetKey(verifier)
}

func (k *verifiersKeystore) SignTx(a iface.Account, tx blockdagtypes.Transaction, chainID *big.Int) (blockdagtypes.Transaction, error) {
	return k.keystore.SignTx(a, tx, chainID)
}

func (k *verifiersKeystore) initVerifiers() error {
	return k.keystore.ScanAccounts()
}

// getPasswords reads the password list from the configured path.
func (k *verifiersKeystore) getPasswords() ([]string, error) {
	dir, err := k.cfg.PasswordsFilePath()
	if err != nil {
		return nil, err
	}
	return makePasswordList(dir)
}

// makeAddress converts an account string (hex address or keystore index) to an Account.
func (k *verifiersKeystore) makeAddress(account string) (iface.Account, error) {
	if common.IsHexAddress(account) {
		return iface.Account{Address: common.HexToAddress(account)}, nil
	}
	index, err := strconv.Atoi(account)
	if err != nil || index < 0 {
		return iface.Account{}, fmt.Errorf("invalid account address or index %q", account)
	}
	slog.Warn("-------------------------------------------------------------------")
	slog.Warn("Referring to accounts by order in the keystore folder is dangerous!")
	slog.Warn("This functionality is deprecated and will be removed in the future!")
	slog.Warn("Please use explicit addresses! (can search via `geth account list`)")
	slog.Warn("-------------------------------------------------------------------")

	accs := k.keystore.Accounts()
	if len(accs) <= index {
		return iface.Account{}, fmt.Errorf("index %d higher than number of accounts %d", index, len(accs))
	}
	return accs[index], nil
}

// getPassPhraseWithList picks a password from the list or prompts the user.
func (k *verifiersKeystore) getPassPhraseWithList(pos int, passwords []string) string {
	if len(passwords) > 0 {
		if pos < len(passwords) {
			return passwords[pos]
		}
		return passwords[len(passwords)-1]
	}
	return k.getPassPhrase(false)
}

// getPassPhrase prompts the user for a password, optionally asking for confirmation.
func (k *verifiersKeystore) getPassPhrase(confirmation bool) string {
	password, err := k.prompter.PromptPassword("Password: ")
	if err != nil {
		slog.Error("Failed to read password", "err", err)
	}
	if confirmation {
		confirm, err := k.prompter.PromptPassword("Repeat password: ")
		if err != nil {
			slog.Error("Failed to read password confirmation", "err", err)
		}
		if password != confirm {
			slog.Error("Passwords do not match")
		}
	}
	return password
}

// unlockAccount tries to unlock the account at the given position up to 3 times.
func (k *verifiersKeystore) unlockAccount(address string, pos int, passwords []string) error {
	account, err := k.makeAddress(address)
	if err != nil {
		slog.Error("Could not resolve account address", "error", err)
		return err
	}
	if k.keystore.IsUnlocked(account.Address) {
		return nil
	}
	for trials := 0; trials < 3; trials++ {
		password := k.getPassPhraseWithList(pos, passwords)
		err = k.keystore.Unlock(account, password)
		if err != nil {
			slog.Warn("Failed to unlock account, try again", "account", address, "error", err)
			continue
		}
		slog.Info("Unlocked account", "address", account.Address.Hex())
		return nil
	}
	return nil
}

// findAccountPosition finds the index of the target address in the accounts list, or -1.
func findAccountPosition(accounts []iface.Account, targetAddress string) int {
	for index, account := range accounts {
		if account.Address.String() == targetAddress {
			return index
		}
	}
	return -1
}

// makePasswordList reads newline-separated passwords from a file.
func makePasswordList(path string) ([]string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		slog.Error("Failed to read password file", "error", err)
		return nil, err
	}
	lines := strings.Split(string(text), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines, nil
}
