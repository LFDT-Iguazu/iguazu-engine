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
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/common"
	"github.com/LFDT-Iguazu/iguazu-types/rlp"
)

const valSyncOpDataMinLen = common.Uint64Size + common.Uint64Size + common.AddressLength + common.HashLength

type VersionValSyncOp uint16

const (
	NoVer VersionValSyncOp = iota
	Ver1
	Ver2
)

type validatorSyncOperation struct {
	version           VersionValSyncOp
	opType            types.ValidatorSyncOp
	initTxHash        common.Hash
	procEpoch         uint64
	index             uint64
	creator           common.Address
	amount            *big.Int
	withdrawalAddress *common.Address
	balance           *big.Int
	activationEpoch   uint64
	exitEpoch         uint64
	stakeAmount       *big.Int
}

// rlpValSyncOpVer2 rlp representation of ValidatorSyncOperation op ver 2.
// Balance of ver 1 is replaced by StakeAmount.
type rlpValSyncOpVer2 struct {
	OpType            types.ValidatorSyncOp
	InitTxHash        common.Hash
	ProcEpoch         uint64
	Index             uint64
	Creator           common.Address
	WithdrawalAddress common.Address
	Amount            big.Int
	ActivationEpoch   uint64
	ExitEpoch         uint64
	StakeAmount       big.Int
}

// rlpValSyncOpVer1 rlp representation of ValidatorSyncOperation op ver 1.
type rlpValSyncOpVer1 struct {
	OpType            types.ValidatorSyncOp
	InitTxHash        common.Hash
	ProcEpoch         uint64
	Index             uint64
	Creator           common.Address
	WithdrawalAddress common.Address
	Amount            big.Int
	Balance           big.Int
}

func (op *validatorSyncOperation) init(
	version VersionValSyncOp,
	opType types.ValidatorSyncOp,
	initTxHash common.Hash,
	procEpoch uint64,
	index uint64,
	creator common.Address,
	amount *big.Int,
	withdrawalAddress *common.Address,
	balance *big.Int,
	activationEpoch uint64,
	exitEpoch uint64,
	stakeAmount *big.Int,
) error {
	if initTxHash == (common.Hash{}) {
		return ErrNoInitTxHash
	}
	if creator == (common.Address{}) {
		return ErrNoCreatorAddress
	}
	if opType == types.UpdateBalance {
		if amount == nil {
			return ErrNoAmount
		}
		if withdrawalAddress == nil {
			return ErrNoWithdrawalAddress
		}
		if version == Ver1 && balance == nil {
			return ErrNoBalance
		}
		if version >= Ver2 && stakeAmount == nil {
			return ErrNoStakeAmount
		}
	}
	op.initTxHash = initTxHash
	op.opType = opType
	op.procEpoch = procEpoch
	op.index = index
	op.creator = creator
	op.amount = amount
	if opType == types.UpdateBalance {
		op.withdrawalAddress = withdrawalAddress
		if version < Ver2 {
			op.balance = balance
		} else {
			op.stakeAmount = stakeAmount
		}
	}
	op.activationEpoch = activationEpoch
	op.exitEpoch = exitEpoch
	//set data version
	op.version = version
	return nil
}

// NewValidatorSyncOperation creates an operation for creating validator sync operation.
func NewValidatorSyncOperation(
	version VersionValSyncOp,
	opType types.ValidatorSyncOp,
	initTxHash common.Hash,
	procEpoch uint64,
	index uint64,
	creator common.Address,
	amount *big.Int,
	withdrawalAddress *common.Address,
	balance *big.Int,
	activationEpoch uint64,
	exitEpoch uint64,
	stakeAmount *big.Int,
) (ValidatorSync, error) {
	op := validatorSyncOperation{}
	if err := op.init(version, opType, initTxHash, procEpoch, index, creator, amount, withdrawalAddress, balance, activationEpoch, exitEpoch, stakeAmount); err != nil {
		return nil, err
	}
	return &op, nil
}

// UnmarshalBinary unmarshals a create operation from byte encoding
func (op *validatorSyncOperation) UnmarshalBinary(b []byte) error {
	version, binData, err := unwrapVersionedData(b)
	if err != nil {
		return op.unmarshalBinaryLegacy(b)
	}
	switch version {
	case Ver1:
		dec := &rlpValSyncOpVer1{}
		err = rlp.DecodeBytes(binData, dec)
		if err != nil {
			return err
		}
		return op.init(version, dec.OpType, dec.InitTxHash, dec.ProcEpoch, dec.Index, dec.Creator, &dec.Amount, &dec.WithdrawalAddress, &dec.Balance, 0, 0, nil)
	case Ver2:
		dec := &rlpValSyncOpVer2{}
		err = rlp.DecodeBytes(binData, dec)
		if err != nil {
			return err
		}
		return op.init(version,
			dec.OpType, dec.InitTxHash, dec.ProcEpoch,
			dec.Index, dec.Creator, &dec.Amount,
			&dec.WithdrawalAddress, nil,
			dec.ActivationEpoch, dec.ExitEpoch,
			&dec.StakeAmount,
		)
	default:
		return ErrOpBadVersion
	}
}

// MarshalBinary marshals a create operation to byte encoding
func (op *validatorSyncOperation) MarshalBinary() ([]byte, error) {
	var (
		binData           []byte
		err               error
		withdrawalAddress = &common.Address{}
		amount            = new(big.Int)
		balance           = new(big.Int)
		stakeAmount       = new(big.Int)
	)
	if op.opType == types.UpdateBalance {
		withdrawalAddress = op.withdrawalAddress
		amount = op.amount
		if op.balance != nil {
			balance = op.balance
		}
		if op.stakeAmount != nil {
			stakeAmount = op.stakeAmount
		}
	}
	switch op.version {
	case NoVer:
		return op.marshalBinaryLegacy()
	case Ver1:
		binData, err = rlp.EncodeToBytes(&rlpValSyncOpVer1{
			OpType:            op.opType,
			InitTxHash:        op.initTxHash,
			ProcEpoch:         op.procEpoch,
			Index:             op.index,
			Creator:           op.creator,
			WithdrawalAddress: *withdrawalAddress,
			Amount:            *amount,
			Balance:           *balance,
		})
	case Ver2:
		binData, err = rlp.EncodeToBytes(&rlpValSyncOpVer2{
			OpType:            op.opType,
			InitTxHash:        op.initTxHash,
			ProcEpoch:         op.procEpoch,
			Index:             op.index,
			Creator:           op.creator,
			WithdrawalAddress: *withdrawalAddress,
			Amount:            *amount,
			ActivationEpoch:   op.ActivationEpoch(),
			ExitEpoch:         op.ExitEpoch(),
			StakeAmount:       *stakeAmount,
		})
	default:
		return nil, ErrOpBadVersion
	}
	if err != nil {
		return nil, err
	}
	return wrapVersionedData(op.version, binData)
}

// UnmarshalBinary unmarshals deprecated validator sync operation from byte encoding.
func (op *validatorSyncOperation) unmarshalBinaryLegacy(b []byte) error {
	if len(b) < valSyncOpDataMinLen {
		return ErrBadDataLen
	}
	startOffset := 0
	endOffset := startOffset + common.Uint64Size
	opType := types.ValidatorSyncOp(binary.BigEndian.Uint64(b[startOffset:endOffset]))

	startOffset = endOffset
	endOffset = startOffset + common.HashLength
	initTxHash := common.BytesToHash(b[startOffset:endOffset])

	startOffset = endOffset
	endOffset = startOffset + common.Uint64Size
	procEpoch := binary.BigEndian.Uint64(b[startOffset:endOffset])

	startOffset = endOffset
	endOffset = startOffset + common.Uint64Size
	index := binary.BigEndian.Uint64(b[startOffset:endOffset])

	startOffset = endOffset
	endOffset = startOffset + common.AddressLength
	creator := common.BytesToAddress(b[startOffset:endOffset])
	var (
		amount     *big.Int
		withdrawal common.Address
	)
	if opType == types.UpdateBalance {
		startOffset = endOffset
		endOffset = startOffset + common.AddressLength
		withdrawal = common.BytesToAddress(b[startOffset:endOffset])

		startOffset = endOffset
		amount = new(big.Int).SetBytes(b[startOffset:])
	}
	return op.init(NoVer, opType, initTxHash, procEpoch, index, creator, amount, &withdrawal, nil, 0, 0, nil)
}

// marshalBinaryLegacy marshals deprecated validator sync operation to byte encoding.
func (op *validatorSyncOperation) marshalBinaryLegacy() ([]byte, error) {
	bin := make([]byte, 0, valSyncOpDataMinLen)

	enc := make([]byte, common.Uint64Size)
	binary.BigEndian.PutUint64(enc, uint64(op.opType))
	bin = append(bin, enc...)

	bin = append(bin, op.initTxHash.Bytes()...)

	enc = make([]byte, common.Uint64Size)
	binary.BigEndian.PutUint64(enc, op.procEpoch)
	bin = append(bin, enc...)

	enc = make([]byte, common.Uint64Size)
	binary.BigEndian.PutUint64(enc, op.index)
	bin = append(bin, enc...)

	bin = append(bin, op.creator.Bytes()...)

	if op.withdrawalAddress != nil {
		bin = append(bin, op.withdrawalAddress.Bytes()...)
	}
	if op.amount != nil {
		bin = append(bin, op.amount.Bytes()...)
	}
	return bin, nil
}

// Code returns op code of a deposit operation
func (op *validatorSyncOperation) OpCode() Code {
	var code Code
	switch op.opType {
	case types.Activate:
		code = ActivateCode
	case types.Deactivate:
		code = DeactivateCode
	case types.UpdateBalance:
		code = UpdateBalanceCode
	}
	return code
}

// Code always returns an empty address
// It's just a stub for the Operation interface.
func (op *validatorSyncOperation) Address() common.Address {
	return common.Address{}
}

func (op *validatorSyncOperation) InitTxHash() common.Hash {
	return op.initTxHash
}

func (op *validatorSyncOperation) OpType() types.ValidatorSyncOp {
	return op.opType
}

func (op *validatorSyncOperation) ProcEpoch() uint64 {
	return op.procEpoch
}

func (op *validatorSyncOperation) Index() uint64 {
	return op.index
}

func (op *validatorSyncOperation) Creator() common.Address {
	return common.BytesToAddress(op.creator.Bytes())
}

func (op *validatorSyncOperation) Amount() *big.Int {
	if op.amount == nil {
		return nil
	}
	if op.OpCode() == UpdateBalanceCode {
		return new(big.Int).Set(op.amount)
	}
	return nil
}

func (op *validatorSyncOperation) WithdrawalAddress() *common.Address {
	if op.withdrawalAddress == nil {
		return nil
	}
	cpy := common.BytesToAddress(op.withdrawalAddress.Bytes())
	return &cpy
}

func (op *validatorSyncOperation) Balance() *big.Int {
	if op.balance == nil {
		return nil
	}
	return new(big.Int).Set(op.balance)
}

func (op *validatorSyncOperation) ActivationEpoch() uint64 {
	return op.activationEpoch
}

func (op *validatorSyncOperation) ExitEpoch() uint64 {
	return op.exitEpoch
}

// StakeAmount returns the part of Amount withdrawn from the stake (the rest is rewards).
func (op *validatorSyncOperation) StakeAmount() *big.Int {
	if op.stakeAmount == nil {
		return nil
	}
	return new(big.Int).Set(op.stakeAmount)
}

func (op *validatorSyncOperation) Version() VersionValSyncOp {
	return op.version
}
func (op *validatorSyncOperation) SetVersion(ver VersionValSyncOp) {
	op.version = ver
}

func (op *validatorSyncOperation) Print() string {
	if op == nil {
		return "{nil}"
	}
	return fmt.Sprintf("{InitTxHash: %#x, OpType: %d, ProcEpoch: %d, Index: %d, Creator: %#x, Amount: %s, "+
		"Balance: %s, WithdrawalAddress: %#x, ActivationEpoch: %d, exitEpoch: %d, StakeAmount: %s, ver: %d}",
		op.initTxHash,
		op.opType,
		op.procEpoch,
		op.index,
		op.creator,
		op.amount.String(),
		op.balance.String(),
		op.withdrawalAddress.Hex(),
		op.activationEpoch,
		op.exitEpoch,
		op.stakeAmount.String(),
		op.version,
	)
}

type rlpVerWrapper struct {
	Version VersionValSyncOp
	Data    []byte
}

func wrapVersionedData(ver VersionValSyncOp, bin []byte) ([]byte, error) {
	return rlp.EncodeToBytes(rlpVerWrapper{
		Version: ver,
		Data:    bin,
	})
}

func unwrapVersionedData(bin []byte) (ver VersionValSyncOp, data []byte, err error) {
	verWrap := &rlpVerWrapper{}
	err = rlp.DecodeBytes(bin, verWrap)
	if err != nil {
		return
	}
	return verWrap.Version, verWrap.Data, nil
}
