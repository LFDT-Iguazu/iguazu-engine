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

/*
Package validatorsync implements functionality for handling ValidatorSync operations,
enabling synchronization of validator-related data and processes.
*/
package validatorsync

import (
	"fmt"
	"log/slog"
	"math"

	"github.com/LFDT-Iguazu/iguazu-engine/validator/operation"
	valStore "github.com/LFDT-Iguazu/iguazu-engine/validator/storage"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types/era"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

// blockchain is the interface required by validatorsync. The common methods
// are defined in iguazu-types so that gwat can implement them directly.
// ValidatorStorage cannot be expressed in iguazu-types (returns valStore.Storage
// from iguazu-engine) and is added here.
type blockchain interface {
	iface.ValidatorChain
	// ValidatorStorage returns the validator storage for this blockchain.
	ValidatorStorage() valStore.Storage
}

// Backend provides the services needed to create and sign validator sync transactions.
type Backend interface {
	BlockChain() blockchain
	// CreateAndSignTx creates and signs an EIP-1559 data transaction with zero
	// gas/fee/value. gwat implements this using types.DynamicFeeTx and the
	// verifiers keystore, keeping all LGPL code out of this package.
	CreateAndSignTx(from, to common.Address, nonce uint64, data []byte) (*types.Transaction, error)
}

// CreateValidatorSyncTx builds and signs a validator sync transaction.
func CreateValidatorSyncTx(
	backend Backend,
	stateBlockHash common.Hash,
	from common.Address,
	slot uint64,
	valSyncOp *types.ValidatorSync,
	nonce uint64,
) (*types.Transaction, error) {
	bc := backend.BlockChain()
	_, err := ValidateCreateTxValidatorSyncOp(bc, stateBlockHash, slot, valSyncOp)
	if err != nil {
		return nil, err
	}
	// collect validator data
	stateHead := bc.GetHeaderByHash(stateBlockHash)
	stateDb, err := bc.StateAt(stateHead.Root)
	if err != nil {
		return nil, err
	}
	validator, err := bc.ValidatorStorage().GetValidator(stateDb, valSyncOp.Creator)
	if err != nil {
		return nil, err
	}

	// get tx.To address
	valStateAddr := bc.Config().ValidatorsStateAddress

	// validate nonce
	nonceFrom := stateDb.GetNonce(from)
	if nonceFrom > nonce {
		return nil, fmt.Errorf("nonce is too low:  fromAddr=%s fromNonce=%d txNonce=%d", from.Hex(), nonceFrom, nonce)
	}
	var withdrawalAddress *common.Address
	if valSyncOp.OpType == types.UpdateBalance {
		wa := validator.GetWithdrawalAddress()
		withdrawalAddress = wa
	}
	opVer := getValSyncVersionBySlot(bc.Config(), slot)

	slog.Info("Validator sync tx data",
		"slot", slot,
		"ver", opVer,
		"op", valSyncOp.Print(),
		"from", from.Hex(),
	)

	valSyncTxData, err := getValSyncTxData(*valSyncOp, withdrawalAddress, opVer)
	if err != nil {
		return nil, err
	}

	return backend.CreateAndSignTx(from, valStateAddr, nonce, valSyncTxData)
}

// ValidateCreateTxValidatorSyncOp validates that a validator sync op is suitable for tx creation.
func ValidateCreateTxValidatorSyncOp(bc blockchain, stateBlockHash common.Hash, slot uint64, valSyncOp *types.ValidatorSync) (bool, error) {
	if valSyncOp == nil {
		return false, fmt.Errorf("validator sync operation failed: nil data")
	}
	stateHead := bc.GetHeaderByHash(stateBlockHash)
	if stateHead == nil {
		return false, fmt.Errorf("validator sync operation failed: state block not found heash=%s", stateBlockHash.Hex())
	}
	if !bc.Config().IsForkSlotDelegate(slot) {
		stateEpoch := bc.GetSlotInfo().SlotToEpoch(stateHead.Slot)
		if valSyncOp.ProcEpoch < stateEpoch {
			return false, fmt.Errorf("validator sync operation failed: outdated epoch ProcEpoch=%d stateEpoch=%d", valSyncOp.ProcEpoch, stateEpoch)
		}
	}

	stateDb, err := bc.StateAt(stateHead.Root)
	if err != nil {
		return false, err
	}
	if !stateDb.IsValidatorAddress(valSyncOp.Creator) {
		return false, fmt.Errorf("validator sync operation failed: address is not validator: %s", valSyncOp.Creator.Hex())
	}
	validator, err := bc.ValidatorStorage().GetValidator(stateDb, valSyncOp.Creator)
	if err != nil {
		return false, err
	}

	switch valSyncOp.OpType {
	case types.Activate:
		if validator.GetActivationEra() < math.MaxUint64 {
			return false, fmt.Errorf("validator sync operation failed: validator already activated")
		}
	case types.Deactivate:
		if validator.GetExitEra() < math.MaxUint64 {
			return false, fmt.Errorf("validator sync operation failed: validator already deactivated")
		}

		isLowExitEpoch := validator.GetActivationEra() >= bc.GetEraInfo().Number()
		if isLowExitEpoch {
			var procEra *era.Era
			if bc.Config().IsForkSlotValSyncProc(slot) {
				procEpoch := bc.GetSlotInfo().SlotToEpoch(slot)
				procEra = bc.EpochToEra(procEpoch)
				isLowExitEpoch = validator.GetActivationEra() > procEra.Number
			} else {
				procEra = bc.EpochToEra(valSyncOp.ProcEpoch)
				isLowExitEpoch = validator.GetActivationEra() >= procEra.Number
			}
			slog.Info("Create validator sync tx: exit: is low epoch",
				"cond", validator.GetActivationEra() > procEra.Number,
				"procEra", procEra,
				"valActivationEra", validator.GetActivationEra(),
				"IsForkSlotValSyncProc", bc.Config().IsForkSlotValSyncProc(slot),
				"slot", slot,
			)
		}

		if isLowExitEpoch {
			return false, fmt.Errorf("validator sync operation failed: exit epoch is too low")
		}
	case types.UpdateBalance:
		if valSyncOp.Amount == nil || valSyncOp.Amount.Sign() == 0 {
			return false, fmt.Errorf("validator sync operation failed: withdrawal amount is required")
		}
		if valSyncOp.Amount.Sign() == -1 {
			return false, fmt.Errorf("validator sync operation failed: withdrawal amount is negative")
		}
		if bc.Config().IsForkSlotValSyncProc(slot) {
			if valSyncOp.StakeAmount == nil {
				return false, fmt.Errorf("validator sync operation failed: stake amount is required")
			}
			if valSyncOp.StakeAmount.Sign() == -1 || valSyncOp.StakeAmount.Cmp(valSyncOp.Amount) > 0 {
				return false, fmt.Errorf("validator sync operation failed: bad stake amount")
			}
		}
	default:
		return false, fmt.Errorf("validator sync operation failed: unknown oparation type %d", valSyncOp.OpType)
	}
	return true, nil
}

func getValSyncTxData(valSyncOp types.ValidatorSync, withdrawal *common.Address, version operation.VersionValSyncOp) ([]byte, error) {
	var (
		op  operation.Operation
		err error
	)
	if op, err = operation.NewValidatorSyncOperation(
		version,
		valSyncOp.OpType,
		valSyncOp.InitTxHash,
		valSyncOp.ProcEpoch,
		valSyncOp.Index,
		valSyncOp.Creator,
		valSyncOp.Amount,
		withdrawal,
		valSyncOp.Balance,
		valSyncOp.ActivationEpoch,
		valSyncOp.ExitEpoch,
		valSyncOp.StakeAmount,
	); err != nil {
		return nil, err
	}
	b, err := operation.EncodeToBytes(op)
	if err != nil {
		slog.Warn("Failed to encode validator sync operation", "err", err)
		return nil, err
	}
	return b, nil
}

// GetPendingValidatorSyncData retrieves currently processable validators sync operations.
func GetPendingValidatorSyncData(bc blockchain) map[common.Hash]*types.ValidatorSync {
	si := bc.GetSlotInfo()
	currEpoch := si.SlotToEpoch(si.CurrentSlot())

	valSyncOps := bc.GetNotProcessedValidatorSyncData()
	vsPending := make(map[common.Hash]*types.ValidatorSync, len(valSyncOps))
	for k, vs := range valSyncOps {
		if vs.TxHash != nil {
			continue
		}
		saved := bc.GetValidatorSyncData(vs.InitTxHash)

		if saved != nil && saved.TxHash != nil {
			continue
		}
		if vs.ProcEpoch == currEpoch {
			vsPending[k] = vs
		} else if bc.Config().IsForkSlotDelegate(si.CurrentSlot()) {
			if vs.ProcEpoch < currEpoch {
				//remove stale validator sync operation from the pool
				if vs.ProcEpoch < bc.Config().ForkSlotDelegate {
					if vs.TxHash == nil {
						//set dummy txHash
						vs.TxHash = &vs.InitTxHash
					}
					bc.SetValidatorSyncData(vs)
					slog.Warn("=== ValidatorSync: GetPendingValidatorSyncData: stale op removed",
						"slot", si.CurrentSlot(),
						"op", vs.Print(),
					)
					continue
				}

				//add to pending
				vsPending[k] = vs
				slog.Info("=== ValidatorSync: GetPendingValidatorSyncData === pending",
					"slot", si.CurrentSlot(),
					"op", vs.Print(),
				)
			}
		}
	}
	return vsPending
}

func getValSyncVersionBySlot(conf *iface.ChainConfig, slot uint64) operation.VersionValSyncOp {
	var ver operation.VersionValSyncOp
	if conf.IsForkSlotDelegate(slot) {
		ver = operation.Ver1
	}
	if conf.IsForkSlotValSyncProc(slot) {
		ver = operation.Ver2
	}
	return ver
}
