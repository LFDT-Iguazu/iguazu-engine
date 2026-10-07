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

	"github.com/LFDT-Iguazu/iguazu-engine/token/operation"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	wfcommon "github.com/LFDT-Iguazu/iguazu-types/common"
)

// Factory implements iface.TokenProcessorFactory using iguazu-engine's Processor.
type Factory struct {
	bc blockchain
}

// NewFactory returns a TokenProcessorFactory backed by iguazu-engine's token processor.
func NewFactory(bc blockchain) *Factory {
	return &Factory{bc: bc}
}

// New creates a Processor for the given block context and state, then wraps it
// in a thin adapter that satisfies iface.TokenProcessor.
func (f *Factory) New(ctx iface.BlockContext, state iface.StateDB) iface.TokenProcessor {
	p := NewProcessor(ctx, state, f.bc)
	return &processorAdapter{inner: p}
}

// processorAdapter adapts *Processor to iface.TokenProcessor.
type processorAdapter struct {
	inner *Processor
}

func (a *processorAdapter) IsToken(addr [20]byte) bool {
	return a.inner.IsToken(wfcommon.Address(addr))
}

func (a *processorAdapter) Call(caller iface.Ref, tokenAddr [20]byte, value *big.Int, op []byte) ([]byte, error) {
	decoded, err := operation.DecodeBytes(op)
	if err != nil {
		return nil, err
	}
	// token.Ref = iface.Ref, so caller can be passed directly.
	return a.inner.Call(caller, wfcommon.Address(tokenAddr), value, decoded)
}
