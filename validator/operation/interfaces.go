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

package operation

import (
	"math/big"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

// Deposit contains all attributes for validator deposit.
type Deposit interface {
	Operation
	PubKey() common.BlsPubKey
	CreatorAddress() common.Address
	WithdrawalAddress() common.Address
	Signature() common.BlsSignature
	DelegatingStake() *DelegatingStakeData
}

// ValidatorSync contains all attributes for validator sync op.
type ValidatorSync interface {
	Operation
	OpType() types.ValidatorSyncOp
	ProcEpoch() uint64
	Index() uint64
	Creator() common.Address
	InitTxHash() common.Hash
	Amount() *big.Int
	WithdrawalAddress() *common.Address
	Balance() *big.Int
	ActivationEpoch() uint64
	ExitEpoch() uint64
	StakeAmount() *big.Int
	Version() VersionValSyncOp
	Print() string
}

type Exit interface {
	Operation
	PubKey() common.BlsPubKey
	CreatorAddress() common.Address
	ExitAfterEpoch() *uint64
	SetExitAfterEpoch(*uint64)
}

type Withdrawal interface {
	Operation
	CreatorAddress() common.Address
	Amount() *big.Int
}

type WithdrawalFromValState interface {
	Operation
}
