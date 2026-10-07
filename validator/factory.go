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

package validator

import (
	"math/big"

	valStore "github.com/LFDT-Iguazu/iguazu-engine/validator/storage"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

// Factory implements iface.ValidatorProcessorFactory using iguazu-engine's Processor.
type Factory struct {
	chain   iface.ValidatorChain
	storage valStore.Storage
}

// NewFactory returns a ValidatorProcessorFactory backed by iguazu-engine's validator processor.
// It builds a valStore.Storage from the subset of chain config fields available in iface.ChainConfig.
func NewFactory(chain iface.ValidatorChain) *Factory {
	cfg := chain.Config()
	addr := cfg.ValidatorsStateAddress
	storage := valStore.NewStorage(&valStore.ChainConfig{
		SlotsPerEpoch:          cfg.SlotsPerEpoch,
		ValidatorsStateAddress: &addr,
		ValidatorsPerSlot:      cfg.ValidatorsPerSlot,
	})
	return &Factory{chain: chain, storage: storage}
}

// New creates a Processor for the given block context and state, then wraps it
// in a thin adapter that satisfies iface.ValidatorProcessor.
func (f *Factory) New(ctx iface.BlockContext, state iface.StateDB) iface.ValidatorProcessor {
	bc := &blockchainAdapter{ValidatorChain: f.chain, storage: f.storage}
	p := NewProcessor(ctx, state, bc)
	return &processorAdapter{inner: p}
}

// blockchainAdapter embeds iface.ValidatorChain and adds ValidatorStorage()
// so the combined type satisfies the validator processor's blockchain interface.
type blockchainAdapter struct {
	iface.ValidatorChain
	storage valStore.Storage
}

func (b *blockchainAdapter) ValidatorStorage() valStore.Storage { return b.storage }

// processorAdapter adapts *Processor to iface.ValidatorProcessor.
type processorAdapter struct {
	inner *Processor
}

func (a *processorAdapter) IsValidatorOp(to *[20]byte) bool {
	if to == nil {
		return false
	}
	addr := common.Address(*to)
	return a.inner.IsValidatorOp(&addr)
}

func (a *processorAdapter) Call(caller iface.Ref, to [20]byte, value *big.Int, data []byte, txHash [32]byte) ([]byte, error) {
	return a.inner.Call(
		validatorCallerRef{caller},
		common.Address(to),
		value,
		&validatorMsg{data: data, txHash: common.Hash(txHash)},
	)
}

func (a *processorAdapter) GetValidatorsStateAddress() [20]byte {
	return [20]byte(a.inner.GetValidatorsStateAddress())
}

func (a *processorAdapter) ProcessRewards(coinbase [20]byte, reward *big.Int) error {
	return a.inner.ProcessRewards(coinbase, reward)
}

// validatorCallerRef adapts iface.Ref to the validator package's Ref interface.
type validatorCallerRef struct{ r iface.Ref }

func (v validatorCallerRef) Address() common.Address { return common.Address(v.r.Address()) }

// validatorMsg satisfies the private message interface in the validator package.
type validatorMsg struct {
	data   []byte
	txHash common.Hash
}

func (m *validatorMsg) Data() []byte        { return m.data }
func (m *validatorMsg) TxHash() common.Hash { return m.txHash }
