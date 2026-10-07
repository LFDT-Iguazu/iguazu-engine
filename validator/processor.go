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
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"

	"github.com/LFDT-Iguazu/iguazu-engine/validator/operation"
	valStore "github.com/LFDT-Iguazu/iguazu-engine/validator/storage"
	"github.com/LFDT-Iguazu/iguazu-engine/validator/txlog"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

var (
	// errors
	ErrTooLowDepositValue = errors.New("deposit value is too low")
	// ErrInsufficientFundsForOp is returned if the transaction sender doesn't
	// have enough funds for transfer(topmost call only).
	ErrInsufficientFundsForOp = errors.New("insufficient funds for op")
	ErrInvalidFromAddresses   = errors.New("withdrawal and sender addresses are mismatch")
	ErrUnknownValidator       = errors.New("unknown validator")
	ErrNoWithdrawalCred       = errors.New("no withdrawal credentials")
	ErrMismatchPulicKey       = errors.New("validator public key mismatch")
	ErrNotActivatedValidator  = errors.New("validator not activated yet")
	ErrValidatorActivated     = errors.New("validator is activated")
	ErrValidatorIsOut         = errors.New("validator is exited")
	ErrInvalidToAddress       = errors.New("address to must be validators state address")
	ErrNoSavedValSyncOp       = errors.New("no coordinated confirmation of validator sync data")
	ErrValSyncTxExists        = errors.New("validator sync tx already exists")
	ErrValOpBlocked           = errors.New("blocked by another validator operation")
	ErrNoActiveWithdrawalOp   = errors.New("no active withdrawal operation")
	ErrMismatchValSyncOp      = errors.New("validator sync tx data is not conforms to coordinated confirmation data")
	ErrInvalidOpEpoch         = errors.New("epoch to apply tx is not acceptable")
	ErrTxNF                   = errors.New("tx not found")
	ErrReceiptNF              = errors.New("receipt not found")
	ErrInvalidReceiptStatus   = errors.New("receipt status is failed")
	ErrInvalidOpCode          = errors.New("invalid operation code")
	ErrInvalidCreator         = errors.New("invalid creator address")
	ErrInvalidAmount          = errors.New("invalid amount")
	ErrMismatchDelegateData   = errors.New("validator deposit failed (mismatch delegate stake data)")
	ErrSenderRejByDelegate    = errors.New("sender addresses rejected by delegating stake rules")
)

const (
	postpone = 2
)

var (
	// minimal value - 10 wat
	MinDepositVal, _ = new(big.Int).SetString("10000000000000000000", 10)
)

// Ref represents caller of the validator processor
type Ref interface {
	Address() common.Address
}

// blockchain is the interface required by the Processor. The common methods
// are defined in iguazu-types so that gwat can implement them directly.
// ValidatorStorage cannot be expressed in iguazu-types (returns valStore.Storage
// which lives in iguazu-engine) and is added here.
type blockchain interface {
	iface.ValidatorChain
	// ValidatorStorage returns the validator storage for this blockchain.
	ValidatorStorage() valStore.Storage
}

type message interface {
	TxHash() common.Hash
	Data() []byte
}

// Processor is a processor of all validator related operations.
// All transaction related operations that mutates state of the validator are called using Call method.
// Methods of the operation name are used for getting state of the validator.
type Processor struct {
	state        iface.StateDB
	ctx          iface.BlockContext
	eventEmmiter *txlog.EventEmmiter
	storage      valStore.Storage
	blockchain   blockchain
}

// NewProcessor creates new validator processor
func NewProcessor(blockCtx iface.BlockContext, stateDb iface.StateDB, bc blockchain) *Processor {
	return &Processor{
		ctx:          blockCtx,
		state:        stateDb,
		eventEmmiter: txlog.NewEventEmmiter(stateDb),
		storage:      bc.ValidatorStorage(),
		blockchain:   bc,
	}
}

func (p *Processor) GetBlockContext() iface.BlockContext {
	return p.ctx
}

func (p *Processor) getDepositCount() uint64 {
	return p.Storage().GetDepositCount(p.state)
}

func (p *Processor) incrDepositCount() {
	p.Storage().IncrementDepositCount(p.state)
}

func (p *Processor) GetValidatorsStateAddress() common.Address {
	valAddress := p.Storage().GetValidatorsStateAddress()
	if valAddress == nil {
		return common.Address{}
	}

	return *valAddress
}

func (p *Processor) Storage() valStore.Storage {
	return p.storage
}

// IsValidatorOp returns true if tx is validator operation
func (p *Processor) IsValidatorOp(addrTo *common.Address) bool {
	if addrTo == nil {
		return false
	}

	return *addrTo == p.GetValidatorsStateAddress()
}

// Call performs all transaction related operations that mutates state of the validator and validators state
//
// The only following operations can be performed using the method:
//   - validator: Deposit
//   - coordinating node: Activate
//   - validator: RequestExit
//   - coordinating node: Deactivate
//
// It returns byte representation of the return value of an operation.
func (p *Processor) Call(caller Ref, toAddr common.Address, value *big.Int, msg message) (ret []byte, err error) {
	op, err := operation.DecodeBytes(msg.Data())
	if err != nil {
		return nil, err
	}

	nonce := p.state.GetNonce(caller.Address())
	p.state.SetNonce(caller.Address(), nonce+1)

	snapshot := p.state.Snapshot()

	ret = nil

	//apply fixes
	if vsOp, ok := op.(operation.ValidatorSync); ok {
		if isApplied, ret, err := p.blockchain.FixValidatorSyncOpProcessing(p, vsOp, msg.TxHash(), caller.Address(), toAddr); isApplied {
			if err != nil {
				p.state.RevertToSnapshot(snapshot)
			}
			return ret, err
		}
	}

	switch v := op.(type) {
	case operation.Deposit:
		ret, err = p.validatorDeposit(caller, toAddr, value, v, msg.TxHash())
		if err != nil {
			slog.Error("Validator deposit: err",
				"opCode", op.OpCode(),
				"tx", msg.TxHash().Hex(),
				"amount", value.String(),
				"from", caller.Address(),
				"creator", v.CreatorAddress().Hex(),
				"withdrawalAddress", v.WithdrawalAddress().Hex(),
				"pubKey", v.PubKey().Hex(),
				"delegatingStake", v.DelegatingStake() != nil,
				"blHash", p.ctx.BlockHash.Hex(),
				"err", err,
			)
		} else {
			slog.Info("Validator deposit: success",
				"opCode", op.OpCode(),
				"tx", msg.TxHash().Hex(),
				"amount", value.String(),
				"from", caller.Address(),
				"creator", v.CreatorAddress().Hex(),
				"withdrawalAddress", v.WithdrawalAddress().Hex(),
				"pubKey", v.PubKey().Hex(),
				"delegatingStake", v.DelegatingStake() != nil,
				"blHash", p.ctx.BlockHash.Hex(),
			)
		}
	case operation.ValidatorSync:
		ret, err = p.syncOpProcessing(v, msg)
		if err != nil {
			slog.Error("Validator sync: err",
				"opCode", op.OpCode(),
				"initTx", v.InitTxHash().Hex(),
				"creator", v.Creator().Hex(),
				"procEpoch", v.ProcEpoch(),
				"blHash", p.ctx.BlockHash.Hex(),
				"err", err,
				"tx", msg.TxHash().Hex(),
			)
		} else {
			slog.Info("Validator sync: success",
				"opCode", op.OpCode(),
				"initTx", v.InitTxHash().Hex(),
				"creator", v.Creator().Hex(),
				"procEpoch", v.ProcEpoch(),
				"blHash", p.ctx.BlockHash.Hex(),
				"tx", msg.TxHash().Hex(),
			)
		}
	case operation.Exit:
		ret, err = p.validatorExit(caller, toAddr, v, msg.TxHash())
		if err != nil {
			slog.Error("Validator exit: err",
				"opCode", op.OpCode(),
				"tx", msg.TxHash().Hex(),
				"creator", v.CreatorAddress().Hex(),
				"exitAfterEpoch", fmt.Sprintf("%d", v.ExitAfterEpoch()),
				"err", err,
			)
		} else {
			slog.Info("Validator exit: success",
				"opCode", op.OpCode(),
				"tx", msg.TxHash().Hex(),
				"creator", v.CreatorAddress().Hex(),
				"exitAfterEpoch", fmt.Sprintf("%d", v.ExitAfterEpoch()),
			)
		}
	case operation.Withdrawal:
		ret, err = p.validatorWithdrawal(caller, toAddr, v, msg.TxHash())
		if err != nil {
			slog.Error("Validator withdrawal: err",
				"opCode", op.OpCode(),
				"tx", msg.TxHash().Hex(),
				"amount", v.Amount().String(),
				"creator", v.CreatorAddress().Hex(),
				"blHash", p.ctx.BlockHash.Hex(),
				"err", err,
			)
		} else {
			slog.Info("Validator withdrawal: success",
				"opCode", op.OpCode(),
				"tx", msg.TxHash().Hex(),
				"amount", v.Amount().String(),
				"creator", v.CreatorAddress().Hex(),
				"blHash", p.ctx.BlockHash.Hex(),
			)
		}
	case operation.WithdrawalFromValState:
		ret, err = p.validatorStateAddressWithdrawal(toAddr)
		if err != nil {
			slog.Error("Validator state address withdrawal: err",
				"opCode", op.OpCode(),
				"tx", msg.TxHash().Hex(),
				"withdrawalAddress", p.blockchain.Config().WaterfallDummyAddress,
				"blHash", p.ctx.BlockHash.Hex(),
				"err", err,
			)
		} else {
			slog.Info("Validator state address withdrawal: success",
				"opCode", op.OpCode(),
				"tx", msg.TxHash().Hex(),
				"withdrawalAddress", p.blockchain.Config().WaterfallDummyAddress,
				"blHash", p.ctx.BlockHash.Hex(),
			)
		}
	}

	if err != nil {
		p.state.RevertToSnapshot(snapshot)
	}

	return ret, err
}

func (p *Processor) validatorDeposit(caller Ref, toAddr common.Address, value *big.Int, op operation.Deposit, txHash common.Hash) (_ []byte, err error) {
	if !p.IsValidatorOp(&toAddr) {
		return nil, ErrInvalidToAddress
	}

	// validate deposit signature
	if err = operation.VerifyDepositSig(op.Signature(), op.PubKey(), op.CreatorAddress(), op.WithdrawalAddress()); err != nil {
		if !p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) {
			return nil, err
		}
		err = operation.VerifyDepositSigWithDelegate(op.Signature(), op.PubKey(), op.CreatorAddress(), op.WithdrawalAddress(), op.DelegatingStake())
		if err != nil {
			return nil, err
		}
	}

	if value == nil || value.Cmp(MinDepositVal) < 0 {
		return nil, ErrTooLowDepositValue
	}

	// check amount can add to log
	if !common.BnCanCastToUint64(new(big.Int).Div(value, common.BigGwei)) {
		return nil, ErrInvalidAmount
	}

	from := caller.Address()

	balanceFrom := p.state.GetBalance(from)
	if balanceFrom.Cmp(value) < 0 {
		return nil, fmt.Errorf("%w: address %v", ErrInsufficientFundsForOp, from.Hex())
	}

	withdrawalAddress := op.WithdrawalAddress()

	validator := valStore.NewValidator(op.PubKey(), op.CreatorAddress(), &withdrawalAddress)

	if op.DelegatingStake() != nil {
		//check activation fork
		if !p.blockchain.Config().IsForkSlotDelegate(p.ctx.Slot) {
			return nil, operation.ErrDelegateForkRequire
		}
		if err = op.DelegatingStake().Rules.Validate(); err != nil {
			return nil, err
		}
		validator.DelegatingStake, err = operation.NewDelegatingStakeData(&op.DelegatingStake().Rules, op.DelegatingStake().TrialPeriod, &op.DelegatingStake().TrialRules)
		if err != nil {
			return nil, err
		}
	}

	// if validator already exist - check data compliance
	currValidator, _ := p.Storage().GetValidator(p.state, op.CreatorAddress())

	if currValidator != nil {
		if currValidator.ActivationEra < math.MaxUint64 {
			return nil, errors.New("validator deposit failed (validator already activated)")
		}

		if currValidator.PubKey != op.PubKey() {
			return nil, errors.New("validator deposit failed (mismatch public key)")
		}

		if *currValidator.WithdrawalAddress != op.WithdrawalAddress() {
			return nil, errors.New("validator deposit failed (mismatch withdrawal address)")
		}

		//check delegating stake data are equal
		curDlg, err := currValidator.DelegatingStake.MarshalBinary()
		if err != nil {
			return nil, err
		}
		opDlg, err := op.DelegatingStake().MarshalBinary()
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(curDlg, opDlg) {
			return nil, ErrMismatchDelegateData
		}

		validator = currValidator
	}

	// ver1 data validation
	if validator.Version() >= valStore.Ver1 {
		//withdrawal operations are limited to 1 op at time
		//if operation already has been requested, check it expiration
		prevOpTx := validator.GetWithdrawalTx()
		if prevOpTx != nil {
			rc, blHash, _ := p.blockchain.GetTransactionReceipt(*prevOpTx)
			//check if prev tx in same block
			if blHash != (common.Hash{}) && blHash == p.ctx.BlockHash {
				slog.Error("Validator deposit: op blocked by withdrawal (op in same block)",
					"blockedByTx", prevOpTx.Hex(),
					"opCode", op.OpCode(),
					"creator", op.CreatorAddress().Hex(),
					"blHash", p.ctx.BlockHash.Hex(),
					"error", ErrValOpBlocked,
				)
				return nil, ErrValOpBlocked
			}
			//check prev op success
			if rc != nil && rc.Status == types.ReceiptStatusSuccessful {
				//check prev operation expiration
				prevHeader := p.blockchain.GetHeaderByHash(blHash)
				expiration := p.blockchain.Config().ValidatorOpExpireSlots
				currSlot := p.ctx.Slot
				if prevHeader != nil && currSlot < prevHeader.Slot+expiration {
					slog.Error("Validator deposit: op blocked by withdrawal",
						"blockedByTx", prevOpTx.Hex(),
						"opCode", op.OpCode(),
						"creator", op.CreatorAddress().Hex(),
						"blHash", p.ctx.BlockHash.Hex(),
						"error", ErrValOpBlocked,
					)
					return nil, ErrValOpBlocked
				}
			}
		}
	}

	//update current validator's data version
	validator = p.updateValidatorVersionBySlot(validator)
	validator.AddDepositTxs(txHash)

	validator.AddStake(from, value)

	err = p.Storage().SetValidator(p.state, validator)
	if err != nil {
		return nil, err
	}

	logData := txlog.PackDepositLogData(op.PubKey(), op.CreatorAddress(), op.WithdrawalAddress(), value, op.Signature(), p.getDepositCount())

	if p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) {
		p.eventEmmiter.Deposit(toAddr, logData, p.eventEmmiter.NewIndexedAddressLogEntry(op.CreatorAddress()))
	} else {
		p.eventEmmiter.Deposit(toAddr, logData)
	}

	p.incrDepositCount()
	// burn value from sender balance
	p.state.SubBalance(from, value)

	return value.FillBytes(make([]byte, 32)), nil
}

func (p *Processor) validatorExit(caller Ref, toAddr common.Address, op operation.Exit, txHash common.Hash) ([]byte, error) {
	if !p.IsValidatorOp(&toAddr) {
		return nil, ErrInvalidToAddress
	}

	from := caller.Address()
	validator, err := p.Storage().GetValidator(p.state, op.CreatorAddress())
	if err != nil {
		return nil, err
	}

	if validator == nil {
		return nil, ErrUnknownValidator
	}

	if validator.GetPubKey() != op.PubKey() {
		return nil, ErrMismatchPulicKey
	}

	if op.ExitAfterEpoch() == nil {
		exitAftEpoch := p.blockchain.GetSlotInfo().SlotToEpoch(p.ctx.Slot) + 1
		op.SetExitAfterEpoch(&exitAftEpoch)
	}

	if p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) && validator.GetActivationEra() >= p.blockchain.GetEraInfo().Number() {
		return nil, ErrNotActivatedValidator
	} else if validator.GetActivationEra() > p.blockchain.GetEraInfo().Number() {
		return nil, ErrNotActivatedValidator
	}

	if validator.GetExitEra() != math.MaxUint64 {
		return nil, ErrValidatorIsOut
	}

	if validator.HasDelegatingStake() {
		//check delegating roles
		//retrieve actual rules
		var actualRules = &validator.DelegatingStake.Rules
		isTrial, err := p.IsValidatorTrialPeriod(validator)
		if err != nil {
			return nil, err
		}
		if isTrial {
			actualRules = &validator.DelegatingStake.TrialRules
		}
		allowedAddrs := actualRules.Exit()
		var isAllowed bool
		for _, adr := range allowedAddrs {
			if adr == from {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			return nil, ErrSenderRejByDelegate
		}
	} else if from != *validator.GetWithdrawalAddress() {
		return nil, ErrInvalidFromAddresses
	}

	// ver1 data validation
	if validator.Version() >= valStore.Ver1 {
		//if operation already has been requested, check it expiration
		prevOpTx := validator.GetExitTx()
		if prevOpTx != nil {
			rc, blHash, _ := p.blockchain.GetTransactionReceipt(*prevOpTx)
			//check if prev tx in same block
			if blHash != (common.Hash{}) && blHash == p.ctx.BlockHash {
				slog.Error("Validator exit: op blocked (op in same block)",
					"blockedByTx", prevOpTx.Hex(),
					"opCode", op.OpCode(),
					"creator", op.CreatorAddress().Hex(),
					"blHash", p.ctx.BlockHash.Hex(),
					"error", ErrValOpBlocked,
				)
				return nil, ErrValOpBlocked
			}
			//check prev op success
			if rc != nil && rc.Status == types.ReceiptStatusSuccessful {
				//check prev operation expiration
				prevHeader := p.blockchain.GetHeaderByHash(blHash)
				expiration := p.blockchain.Config().ValidatorOpExpireSlots
				currSlot := p.ctx.Slot
				if prevHeader != nil && currSlot < prevHeader.Slot+expiration {
					slog.Error("Validator exit: op blocked",
						"blockedByTx", prevOpTx.Hex(),
						"opCode", op.OpCode(),
						"creator", op.CreatorAddress().Hex(),
						"blHash", p.ctx.BlockHash.Hex(),
						"error", ErrValOpBlocked,
					)
					return nil, ErrValOpBlocked
				}
			}
		}
	}

	//update current validator's data version
	validator = p.updateValidatorVersionBySlot(validator)
	validator.SetExitTx(&txHash)

	// update validator
	err = p.Storage().SetValidator(p.state, validator)
	if err != nil {
		return nil, err
	}

	logData := txlog.PackExitRequestLogData(op.PubKey(), op.CreatorAddress(), validator.GetIndex(), op.ExitAfterEpoch())

	if p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) {
		p.eventEmmiter.ExitRequest(toAddr, logData, p.eventEmmiter.NewIndexedAddressLogEntry(op.CreatorAddress()))
	} else {
		p.eventEmmiter.ExitRequest(toAddr, logData)
	}

	return op.CreatorAddress().Bytes(), nil
}

func (p *Processor) validatorWithdrawal(caller Ref, toAddr common.Address, op operation.Withdrawal, txHash common.Hash) ([]byte, error) {
	if !p.IsValidatorOp(&toAddr) {
		return nil, ErrInvalidToAddress
	}

	// check amount can add to log
	opAmount := new(big.Int).Set(op.Amount())
	if !common.BnCanCastToUint64(new(big.Int).Div(opAmount, common.BigGwei)) {
		return nil, ErrInvalidAmount
	}

	from := caller.Address()
	validator, err := p.Storage().GetValidator(p.state, op.CreatorAddress())
	if err != nil {
		return nil, err
	}

	if validator == nil {
		return nil, ErrUnknownValidator
	}

	// ver1 data validation
	if validator.Version() >= valStore.Ver1 {
		//withdrawal operations are limited to 1 op at time
		//if operation already has been requested, check it expiration
		prevOpTx := validator.GetWithdrawalTx()
		if prevOpTx != nil {
			rc, blHash, _ := p.blockchain.GetTransactionReceipt(*prevOpTx)
			//check if prev tx in same block
			if blHash != (common.Hash{}) && blHash == p.ctx.BlockHash {
				slog.Error("Validator withdrawal: op blocked (op in same block)",
					"blockedByTx", prevOpTx.Hex(),
					"opCode", op.OpCode(),
					"amount", opAmount.String(),
					"creator", op.CreatorAddress().Hex(),
					"blHash", p.ctx.BlockHash.Hex(),
					"error", ErrValOpBlocked,
				)
				return nil, ErrValOpBlocked
			}

			//check prev op success
			if rc != nil && rc.Status == types.ReceiptStatusSuccessful {
				//check prev operation expiration
				prevHeader := p.blockchain.GetHeaderByHash(blHash)
				expiration := p.blockchain.Config().ValidatorOpExpireSlots
				currSlot := p.ctx.Slot
				if prevHeader != nil && currSlot < prevHeader.Slot+expiration {
					slog.Error("Validator withdrawal: op blocked",
						"blockedByTx", prevOpTx.Hex(),
						"opCode", op.OpCode(),
						"amount", opAmount.String(),
						"creator", op.CreatorAddress().Hex(),
						"error", ErrValOpBlocked,
						"blHash", p.ctx.BlockHash.Hex(),
					)
					return nil, ErrValOpBlocked
				}
			}
		}
	}
	//update current validator's data version
	validator = p.updateValidatorVersionBySlot(validator)
	validator.SetWithdrawalTx(&txHash)

	// if total deposited amount is less than the effective balance
	// - deposit is insufficient to activate validator.
	effectiveBalanceWei := new(big.Int).Mul(p.blockchain.Config().EffectiveBalance, common.BigWat)
	if stake := validator.TotalStake(); validator.GetActivationEra() == math.MaxUint64 &&
		stake != nil && stake.Cmp(effectiveBalanceWei) < 0 {
		// withdrawal of insufficient deposit to activate validator
		slog.Info("Validator withdrawal: refunds of insufficient deposit",
			"opCode", op.OpCode(),
			"amount", opAmount.String(),
			"creator", op.CreatorAddress().Hex(),
			"blHash", p.ctx.BlockHash.Hex(),
		)
		stakeByAddr := validator.StakeByAddress(from)
		//withdrawal address must be one of the depositors
		if stakeByAddr == nil || stakeByAddr.Cmp(new(big.Int)) == 0 {
			return nil, ErrInvalidFromAddresses
		}
		// check amount
		if stakeByAddr.Cmp(opAmount) < 0 {
			return nil, ErrInsufficientFundsForOp
		}
		// if opAmount == 0 - refunds full deposit amount
		if opAmount.Cmp(common.Big0) == 0 {
			opAmount = new(big.Int).Set(stakeByAddr)
		}
		if !p.blockchain.Config().IsForkSlotValOpTracking(p.ctx.Slot) {
			// withdrawal amount from deposit balance
			newStake, err := validator.SubtractStake(from, opAmount)
			if err != nil {
				slog.Error("Validator withdrawal: refunds of insufficient deposit failed",
					"opCode", op.OpCode(),
					"amount", opAmount.String(),
					"creator", op.CreatorAddress().Hex(),
					"blHash", p.ctx.BlockHash.Hex(),
					"error", err,
				)
				return nil, err
			}
			// rm validator stake if empty
			if newStake != nil && newStake.Cmp(common.Big0) == 0 {
				validator.RmStakeByAddress(from)
			}
		}
	} else if validator.HasDelegatingStake() {
		//check delegating roles
		//retrieve actual rules
		var actualRules = &validator.DelegatingStake.Rules
		isTrial, err := p.IsValidatorTrialPeriod(validator)
		if err != nil {
			return nil, err
		}
		if isTrial {
			actualRules = &validator.DelegatingStake.TrialRules
		}
		allowedAddrs := actualRules.Withdrawal()
		var isAllowed bool
		for _, adr := range allowedAddrs {
			if adr == from {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			return nil, ErrSenderRejByDelegate
		}
	} else {
		// check withdrawal credentials
		if from != *validator.GetWithdrawalAddress() {
			return nil, ErrInvalidFromAddresses
		}
	}
	// update validator
	err = p.Storage().SetValidator(p.state, validator)
	if err != nil {
		return nil, err
	}

	// create tx log
	amtGwei := new(big.Int).Div(opAmount, common.BigGwei).Uint64()
	logData := txlog.PackWithdrawalLogData(validator.GetPubKey(), op.CreatorAddress(), validator.GetIndex(), amtGwei)

	if p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) {
		p.eventEmmiter.WithdrawalRequest(
			toAddr,
			logData,
			p.eventEmmiter.NewIndexedAddressLogEntry(op.CreatorAddress()),
		)
	} else {
		p.eventEmmiter.WithdrawalRequest(toAddr, logData)
	}

	return op.CreatorAddress().Bytes(), nil
}

func (p *Processor) validatorStateAddressWithdrawal(toAddr common.Address) ([]byte, error) {
	if !p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) {
		return nil, operation.ErrValSyncForkRequire
	}

	if !p.IsValidatorOp(&toAddr) {
		return nil, ErrInvalidToAddress
	}

	var valsStateBalanceErr, allocBalanceErr error

	valsStateBalance := p.state.GetBalance(toAddr)
	if valsStateBalance.Cmp(big.NewInt(0)) <= 0 {
		valsStateBalanceErr = ErrInsufficientFundsForOp
	}

	allocBalance := p.state.GetBalance(p.blockchain.Config().AllocationContractAddress)
	if allocBalance.Cmp(big.NewInt(0)) <= 0 {
		allocBalanceErr = ErrInsufficientFundsForOp
	}

	if valsStateBalanceErr != nil && allocBalanceErr != nil {
		return nil, ErrInsufficientFundsForOp
	}

	if valsStateBalanceErr == nil {
		p.state.SubBalance(toAddr, valsStateBalance)
		p.state.AddBalance(p.blockchain.Config().WaterfallDummyAddress, valsStateBalance)
	}

	if allocBalanceErr == nil {
		p.state.SubBalance(p.blockchain.Config().AllocationContractAddress, allocBalance)
		p.state.AddBalance(p.blockchain.Config().WaterfallDummyAddress, allocBalance)
	}

	return p.blockchain.Config().WaterfallDummyAddress.Bytes(), nil
}

func (p *Processor) syncOpProcessing(op operation.ValidatorSync, msg message) (ret []byte, err error) {
	if err = ValidateValidatorSyncOp(p.blockchain, op, p.ctx.Slot, msg.TxHash()); err != nil {
		slog.Error("Invalid validator sync op",
			"error", err,
			"OpType", op.OpType(),
			"OpCode", op.OpCode(),
			"Creator", op.Creator().Hex(),
			"InitTxHash", op.InitTxHash().Hex(),
			"blHash", p.ctx.BlockHash.Hex(),
		)
		return nil, err
	}

	switch op.OpCode() {
	case operation.ActivateCode:
		ret, err = p.validatorActivate(op)
	case operation.DeactivateCode:
		ret, err = p.validatorDeactivate(op)
	case operation.UpdateBalanceCode:
		ret, err = p.validatorUpdateBalance(op)
	}

	return ret, err
}

func (p *Processor) validatorActivate(op operation.ValidatorSync) ([]byte, error) {
	validator, err := p.Storage().GetValidator(p.state, op.Creator())
	if err != nil {
		return nil, err
	}
	if validator == nil {
		return nil, ErrUnknownValidator
	}

	if p.blockchain.Config().IsForkSlotValOpTracking(p.ctx.Slot) && validator.GetActivationEra() != math.MaxUint64 {
		return nil, ErrValidatorActivated
	}

	//update current validator's data version
	validator = p.updateValidatorVersionBySlot(validator)
	validator.ResetDepositTxs()

	opEra := p.blockchain.EpochToEra(op.ProcEpoch())
	opEraNr := opEra.Number
	if p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) {
		//use current block era
		opEraNr = p.ctx.Era
	}
	validator.SetActivationEra(opEraNr + postpone)
	validator.SetIndex(op.Index())
	validator.UnsetStake()
	err = p.Storage().SetValidator(p.state, validator)
	if err != nil {
		return nil, err
	}

	p.Storage().AddValidatorToList(p.state, op.Index(), op.Creator())

	//check delegating activation fork
	if p.blockchain.Config().IsForkSlotDelegate(p.ctx.Slot) {
		//add tx log
		logData, err := txlog.PackActivateLogData(op.InitTxHash(), op.Creator(), op.ProcEpoch(), op.Index())
		if err != nil {
			return nil, err
		}
		p.eventEmmiter.AddActivateLog(p.GetValidatorsStateAddress(), logData, op.Creator(), op.InitTxHash())
	}
	return op.Creator().Bytes(), nil
}

func (p *Processor) validatorDeactivate(op operation.ValidatorSync) ([]byte, error) {
	validator, err := p.Storage().GetValidator(p.state, op.Creator())
	if err != nil {
		return nil, err
	}
	if validator == nil {
		return nil, ErrUnknownValidator
	}
	if validator.GetExitEra() != math.MaxUint64 {
		return nil, ErrValidatorIsOut
	}

	opEra := p.blockchain.EpochToEra(op.ProcEpoch())
	opEraNr := opEra.Number
	if p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) {
		opEraNr = p.ctx.Era
	}
	if validator.GetActivationEra() >= opEraNr {
		return nil, ErrNotActivatedValidator
	}

	//update current validator's data version
	validator = p.updateValidatorVersionBySlot(validator)
	validator.SetExitTx(nil)

	exitEra := opEraNr + postpone
	validator.SetExitEra(exitEra)
	err = p.Storage().SetValidator(p.state, validator)
	if err != nil {
		return nil, err
	}

	//check delegating activation fork
	if p.blockchain.Config().IsForkSlotDelegate(p.ctx.Slot) {
		//add tx log
		logData, err := txlog.PackDeactivateLogData(op.InitTxHash(), op.Creator(), op.ProcEpoch(), op.Index())
		if err != nil {
			return nil, err
		}
		p.eventEmmiter.AddDeactivateLog(p.GetValidatorsStateAddress(), logData, op.Creator(), op.InitTxHash())
	}

	return op.Creator().Bytes(), nil
}

// checkAndFixActivationStatus checks and fix activity status by op data while update balance operation.
func (p *Processor) checkAndFixActivationStatus(op operation.ValidatorSync) error {
	if op.Version() < operation.Ver2 {
		return nil
	}

	validator, err := p.Storage().GetValidator(p.state, op.Creator())
	if err != nil {
		return err
	}
	if validator == nil {
		return ErrUnknownValidator
	}

	currEpoch := p.blockchain.GetSlotInfo().SlotToEpoch(p.ctx.Slot)

	//check and fix activation
	if validator.GetActivationEra() == math.MaxUint64 &&
		op.ActivationEpoch() < math.MaxUint64 &&
		currEpoch > op.ActivationEpoch() &&
		op.ActivationEpoch() > 0 {
		activationSlot, err := p.blockchain.GetSlotInfo().SlotOfEpochStart(op.ActivationEpoch())
		if err != nil {
			return err
		}
		// check is activation is expired
		fixActivation := p.ctx.Slot >= activationSlot+p.blockchain.Config().ValidatorOpExpireSlots

		slog.Info("Validator fix activation: run",
			"fixActivation", fixActivation,
			"activationSlot", activationSlot,
			"expireSlots", p.blockchain.Config().ValidatorOpExpireSlots,
			"blSlot", p.ctx.Slot,
			"blEra", p.ctx.Era,
			"blHash", p.ctx.BlockHash.Hex(),
			"op", op.Print(),
		)

		//run activation procedure
		if fixActivation {
			//update current validator's data version
			validator = p.updateValidatorVersionBySlot(validator)
			validator.ResetDepositTxs()

			activateEra := p.ctx.Era + postpone
			validator.SetActivationEra(activateEra)
			validator.SetIndex(op.Index())
			validator.UnsetStake()
			err = p.Storage().SetValidator(p.state, validator)
			if err != nil {
				slog.Error("Validator fix activation: set validator failed",
					"fixActivation", fixActivation,
					"blHash", p.ctx.BlockHash.Hex(),
					"op", op.Print(),
					"err", err,
				)
				return err
			}

			p.Storage().AddValidatorToList(p.state, op.Index(), op.Creator())

			//add tx log
			logData, err := txlog.PackActivateLogData(op.InitTxHash(), op.Creator(), op.ProcEpoch(), op.Index())
			if err != nil {
				slog.Error("Validator fix activation: pack log data failed",
					"fixActivation", fixActivation,
					"blHash", p.ctx.BlockHash.Hex(),
					"op", op.Print(),
					"err", err,
				)
				return err
			}
			p.eventEmmiter.AddActivateLog(p.GetValidatorsStateAddress(), logData, op.Creator(), op.InitTxHash())

			slog.Info("Validator fix activation: success",
				"fixActivation", fixActivation,
				"activateEra", activateEra,
				"blHash", p.ctx.BlockHash.Hex(),
				"op", op.Print(),
			)
		}
	}

	//check and fix deactivation
	if validator.GetExitEra() == math.MaxUint64 &&
		op.ExitEpoch() < math.MaxUint64 && currEpoch > op.ExitEpoch() &&
		validator.GetActivationEra() < p.ctx.Era &&
		op.ExitEpoch() > 0 {
		exitSlot, err := p.blockchain.GetSlotInfo().SlotOfEpochStart(op.ExitEpoch())
		if err != nil {
			return err
		}
		// check is activation is expired
		fixDeactivation := p.ctx.Slot >= exitSlot+p.blockchain.Config().ValidatorOpExpireSlots

		slog.Info("Validator fix deactivation: run",
			"fixDeactivation", fixDeactivation,
			"exitSlot", exitSlot,
			"expireSlots", p.blockchain.Config().ValidatorOpExpireSlots,
			"blSlot", p.ctx.Slot,
			"blEra", p.ctx.Era,
			"blHash", p.ctx.BlockHash.Hex(),
			"op", op.Print(),
		)

		//run activation procedure
		if fixDeactivation {
			//update current validator's data version
			validator = p.updateValidatorVersionBySlot(validator)
			validator.SetExitTx(nil)

			exitEra := p.ctx.Era + postpone
			validator.SetExitEra(exitEra)
			err = p.Storage().SetValidator(p.state, validator)
			if err != nil {
				slog.Error("Validator fix deactivation: set validator failed",
					"fixDeactivation", fixDeactivation,
					"blHash", p.ctx.BlockHash.Hex(),
					"op", op.Print(),
					"err", err,
				)
				return err
			}

			//add tx log
			logData, err := txlog.PackDeactivateLogData(op.InitTxHash(), op.Creator(), op.ProcEpoch(), op.Index())
			if err != nil {
				slog.Error("Validator fix deactivation: pack log data failed",
					"fixDeactivation", fixDeactivation,
					"blHash", p.ctx.BlockHash.Hex(),
					"op", op.Print(),
					"err", err,
				)
				return err
			}
			p.eventEmmiter.AddDeactivateLog(p.GetValidatorsStateAddress(), logData, op.Creator(), op.InitTxHash())

			slog.Info("Validator fix deactivation: success",
				"fixDeactivation", fixDeactivation,
				"exitEra", exitEra,
				"blHash", p.ctx.BlockHash.Hex(),
				"op", op.Print(),
			)
		}
	}

	return nil
}

func (p *Processor) validatorUpdateBalance(op operation.ValidatorSync) ([]byte, error) {
	if p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) {
		// fixes lost activate/deactivate operations
		p.checkAndFixActivationStatus(op)
	}

	valAddress := op.Creator()
	validator, err := p.Storage().GetValidator(p.state, valAddress)
	if err != nil {
		return nil, err
	}
	if validator == nil {
		return nil, ErrUnknownValidator
	}

	// ver1 data validation
	if validator.Version() >= valStore.Ver1 {
		//withdrawal operations are limited to 1 op at time
		prevOpTx := validator.GetWithdrawalTx()
		if prevOpTx == nil {
			//check withdrawal initialized before ForkSlotValOpTracking
			//quick check
			expiration := p.blockchain.Config().ValidatorOpExpireSlots
			var checkSlot uint64
			if p.ctx.Slot > expiration {
				checkSlot = p.ctx.Slot - expiration
			}
			if p.blockchain.Config().IsForkSlotValOpTracking(checkSlot) {
				return nil, ErrNoActiveWithdrawalOp
			}
			//check slot of initTx applying (for transition period only)
			rc, blHash, _ := p.blockchain.GetTransactionReceipt(op.InitTxHash())
			//check initTx success
			if rc == nil {
				return nil, fmt.Errorf("initTx receipt not found: %#x", op.InitTxHash())
			}
			if rc.Status != types.ReceiptStatusSuccessful {
				return nil, fmt.Errorf("initTx receipt status is failed: %#x", op.InitTxHash())
			}
			initTxHeader := p.blockchain.GetHeaderByHash(blHash)
			if p.blockchain.Config().IsForkSlotValOpTracking(initTxHeader.Slot) {
				return nil, ErrNoActiveWithdrawalOp
			}
		} else if *prevOpTx != op.InitTxHash() {
			//if another operation already has been requested, check it expiration
			rc, blHash, _ := p.blockchain.GetTransactionReceipt(*prevOpTx)
			//check prev op success
			if rc != nil && rc.Status == types.ReceiptStatusSuccessful {
				//check prev operation expiration
				prevHeader := p.blockchain.GetHeaderByHash(blHash)
				expiration := p.blockchain.Config().ValidatorOpExpireSlots
				currSlot := p.ctx.Slot
				if prevHeader != nil && currSlot < prevHeader.Slot+expiration {
					slog.Error("Validator withdrawal: op blocked",
						"blockedByTx", prevOpTx.Hex(),
						"opCode", op.OpCode(),
						"amount", op.Amount().String(),
						"creator", op.Creator().Hex(),
						"blHash", p.ctx.BlockHash.Hex(),
						"error", ErrValOpBlocked,
					)
					return nil, ErrValOpBlocked
				}
			}
		}
	}

	//update current validator's data version
	validator = p.updateValidatorVersionBySlot(validator)
	//reset current withdrawal op
	validator.SetWithdrawalTx(nil)

	var withdrawalTo *common.Address
	// if total deposited amount is less than the effective balance
	// - deposit is insufficient to activate validator.
	effectiveBalanceWei := new(big.Int).Mul(p.blockchain.Config().EffectiveBalance, common.BigWat)
	if stake := validator.TotalStake(); validator.GetActivationEra() == math.MaxUint64 &&
		stake != nil && stake.Cmp(effectiveBalanceWei) < 0 {
		// Handle of refund of deposited amount in case of insufficient amount to activate validator
		slog.Info("Validator update balance: refunds of insufficient deposit",
			"opCode", op.OpCode(),
			"InitTxHash", op.InitTxHash().Hex(),
			"amount", op.Amount().String(),
			"procEpoch", op.ProcEpoch(),
			"vIndex", op.Index(),
			"creator", op.Creator().Hex(),
			"blHash", p.ctx.BlockHash.Hex(),
		)
		// check initial tx
		initTx, _, _ := p.blockchain.GetTransaction(op.InitTxHash())
		if initTx == nil {
			return nil, ErrTxNF
		}
		// check init tx data
		iop, err := operation.DecodeBytes(initTx.Data)
		if err != nil {
			slog.Error("can`t unmarshal validator sync operation from tx data", "blHash", p.ctx.BlockHash.Hex(), "err", err)
			return nil, err
		}
		if iop.OpCode() != operation.WithdrawalCode {
			return nil, ErrInvalidOpCode
		}
		// withdrawal to sender of initial tx
		iTxFrom, err := p.blockchain.GetTransactionSender(op.InitTxHash())
		if err != nil {
			return nil, fmt.Errorf("get transaction sender: %w", err)
		}
		withdrawalTo = &iTxFrom

		if p.blockchain.Config().IsForkSlotValOpTracking(p.ctx.Slot) {
			// withdrawal amount from deposit balance
			wDepositAmt := op.Amount()
			curDepositStake := validator.StakeByAddress(iTxFrom)
			if curDepositStake.Cmp(wDepositAmt) < 0 {
				slog.Warn("Validator update balance: refunds of insufficient deposit: op amount less than stake",
					"opCode", op.OpCode(),
					"InitTxHash", op.InitTxHash().Hex(),
					"amount", op.Amount().String(),
					"stake", curDepositStake.String(),
					"procEpoch", op.ProcEpoch(),
					"vIndex", op.Index(),
					"creator", op.Creator().Hex(),
					"blHash", p.ctx.BlockHash.Hex(),
				)
				wDepositAmt = new(big.Int).Set(curDepositStake)
			}

			newStake, err := validator.SubtractStake(iTxFrom, wDepositAmt)
			if err != nil {
				slog.Error("Validator withdrawal: refunds of insufficient deposit failed",
					"opCode", op.OpCode(),
					"InitTxHash", op.InitTxHash().Hex(),
					"amount", op.Amount().String(),
					"stake", curDepositStake.String(),
					"procEpoch", op.ProcEpoch(),
					"vIndex", op.Index(),
					"creator", op.Creator().Hex(),
					"blHash", p.ctx.BlockHash.Hex(),
					"error", err,
				)
				return nil, err
			}
			// rm validator stake if empty
			if newStake != nil && newStake.Cmp(common.Big0) == 0 {
				validator.RmStakeByAddress(iTxFrom)
			}
		}
	} else if validator.HasDelegatingStake() {
		if p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) {
			// update validator data
			err = p.Storage().SetValidator(p.state, validator)
			if err != nil {
				return nil, err
			}
		}
		// Handle delegate rules
		return p.applyDelegatingStakeRules(op, validator)
	} else {
		// Handle default withdrawal op
		// set withdrawal credentials
		withdrawalTo = validator.GetWithdrawalAddress()
		if withdrawalTo == nil {
			return nil, ErrNoWithdrawalCred
		}
	}
	// update validator
	err = p.Storage().SetValidator(p.state, validator)
	if err != nil {
		return nil, err
	}

	// transfer amount to withdrawal address
	p.state.AddBalance(*withdrawalTo, op.Amount())

	//check delegating activation fork
	if p.blockchain.Config().IsForkSlotDelegate(p.ctx.Slot) {
		//add tx log
		logData, err := txlog.PackUpdateBalanceLogData(op.InitTxHash(), op.Creator(), op.ProcEpoch(), op.Amount())
		if err != nil {
			return nil, err
		}
		p.eventEmmiter.AddUpdateBalanceLog(p.GetValidatorsStateAddress(), logData, op.Creator(), op.InitTxHash(), withdrawalTo)
	}

	return op.Creator().Bytes(), nil
}

func (p *Processor) applyDelegatingStakeRules(op operation.ValidatorSync, validator *valStore.Validator) ([]byte, error) {
	slog.Info("Validator update balance: apply delegate rules: start",
		"slot", p.ctx.Slot,
		"op", op.Print(),
		"blHash", p.ctx.BlockHash.Hex(),
	)

	bc := p.blockchain
	//check delegating activation fork
	if !bc.Config().IsForkSlotDelegate(p.ctx.Slot) {
		return nil, operation.ErrDelegateForkRequire
	}

	//retrieve actual rules
	var actualRules = &validator.DelegatingStake.Rules
	isTrial, err := p.IsValidatorTrialPeriod(validator)
	if err != nil {
		return nil, err
	}
	if isTrial {
		actualRules = &validator.DelegatingStake.TrialRules
	}

	opBalance := new(big.Int).Add(op.Balance(), op.Amount())

	//Define withdrawals amounts of profit and stake
	effectiveBalanceWei := new(big.Int).Mul(p.blockchain.Config().EffectiveBalance, common.BigWat)
	profitBalance := new(big.Int).Sub(opBalance, effectiveBalanceWei)
	if profitBalance.Sign() < 0 {
		profitBalance = new(big.Int)
	}

	stakeOpAmt := new(big.Int)
	profitOpAmt := new(big.Int).Sub(profitBalance, op.Amount())
	if profitOpAmt.Sign() <= 0 {
		profitOpAmt = new(big.Int).Set(profitBalance)
		stakeOpAmt = new(big.Int).Sub(op.Amount(), profitOpAmt)
	}

	// calculate share
	upBalInfo := make(txlog.DelegatingStakeLogData, 0, len(actualRules.StakeShare())+len(actualRules.ProfitShare()))
	var percent *big.Int
	if profitOpAmt.Sign() > 0 {
		percent = new(big.Int).Div(profitOpAmt, big.NewInt(100))
		for adr, share := range actualRules.ProfitShare() {
			amt := new(big.Int).Mul(percent, big.NewInt(int64(share)))
			upBalInfo = append(upBalInfo, &txlog.ShareRuleApplying{
				Address:  adr,
				RuleType: txlog.ProfitShare,
				IsTrial:  isTrial,
				Amount:   amt,
			})

			slog.Info("Validator update balance: apply delegate rules: up balance: ProfitShare",
				"adr", adr.Hex(),
				"isTrial", isTrial,
				"Amount", amt.String(),
				"InitTxHash", op.InitTxHash().Hex(),
				"creator", op.Creator().Hex(),
				"blHash", p.ctx.BlockHash.Hex(),
			)

			// transfer amount to withdrawal address
			p.state.AddBalance(adr, amt)
		}
	}
	if stakeOpAmt.Sign() > 0 {
		percent = new(big.Int).Div(stakeOpAmt, big.NewInt(100))
		for adr, share := range actualRules.StakeShare() {
			amt := new(big.Int).Mul(percent, big.NewInt(int64(share)))
			upBalInfo = append(upBalInfo, &txlog.ShareRuleApplying{
				Address:  adr,
				RuleType: txlog.StakeShare,
				IsTrial:  isTrial,
				Amount:   amt,
			})

			slog.Info("Validator update balance: apply delegate rules: up balance: StakeShare",
				"adr", adr.Hex(),
				"isTrial", isTrial,
				"Amount", amt.String(),
				"InitTxHash", op.InitTxHash().Hex(),
				"creator", op.Creator().Hex(),
				"blHash", p.ctx.BlockHash.Hex(),
			)

			// transfer amount to withdrawal address
			p.state.AddBalance(adr, amt)
		}
	}

	//add update balance tx log
	logData, err := txlog.PackUpdateBalanceLogData(op.InitTxHash(), op.Creator(), op.ProcEpoch(), op.Amount())
	if err != nil {
		return nil, err
	}
	p.eventEmmiter.AddUpdateBalanceLog(p.GetValidatorsStateAddress(), logData, op.Creator(), op.InitTxHash(), nil)
	//add delegate stake tx log
	logData, err = txlog.PackDelegatingStakeLogData(upBalInfo)
	if err != nil {
		return nil, err
	}
	p.eventEmmiter.AddDelegatingStakeLog(p.GetValidatorsStateAddress(), logData, upBalInfo.Topics())

	return op.Creator().Bytes(), nil
}

func (p *Processor) IsValidatorTrialPeriod(validator *valStore.Validator) (bool, error) {
	// if validator is not activated yet - trial period
	if validator.GetActivationEra() > p.ctx.Era {
		return true, nil
	}
	bc := p.blockchain
	activationEra := bc.ReadEra(validator.GetActivationEra())
	activationSlot, err := bc.GetSlotInfo().SlotOfEpochStart(activationEra.From)
	if err != nil {
		return false, err
	}
	endTrialSlot := activationSlot + validator.DelegatingStake.TrialPeriod
	if endTrialSlot >= p.ctx.Slot {
		return true, nil
	}
	// if validator deactivated while trial period - trial period
	if validator.GetExitEra() <= p.ctx.Era {
		exitEra := bc.ReadEra(validator.GetExitEra())
		exitSlot, err := bc.GetSlotInfo().SlotOfEpochStart(exitEra.From)
		if err != nil {
			return false, err
		}
		if exitSlot <= endTrialSlot {
			return true, nil
		}
	}
	return false, nil
}

// ValidateValidatorSyncOp validate validator sync op data with context of apply.
func ValidateValidatorSyncOp(bc blockchain, valSyncOp operation.ValidatorSync, applySlot uint64, txHash common.Hash) error {
	savedValSync := bc.GetValidatorSyncData(valSyncOp.InitTxHash())
	if savedValSync == nil {
		return ErrNoSavedValSyncOp
	}

	isForkDelegate := bc.Config().IsForkSlotDelegate(applySlot)
	if !isForkDelegate {
		blockEpoch := bc.GetSlotInfo().SlotToEpoch(applySlot)
		if blockEpoch > valSyncOp.ProcEpoch() {
			return ErrInvalidOpEpoch
		}
	}
	if !CompareValSync(savedValSync, valSyncOp, isForkDelegate) {
		return ErrMismatchValSyncOp
	}
	if savedValSync.TxHash != nil && *savedValSync.TxHash != txHash {
		if !isForkDelegate {
			return ErrValSyncTxExists
		}
		// check tx status
		rc, _, _ := bc.GetTransactionReceipt(*savedValSync.TxHash)
		if rc != nil && rc.Status == types.ReceiptStatusSuccessful {
			return ErrValSyncTxExists
		}
	}

	// check is op initialised by slashing
	if bytes.Equal(valSyncOp.InitTxHash().Bytes()[:common.AddressLength], valSyncOp.Creator().Bytes()) {
		return nil
	}

	// check initial tx
	initTx, _, _ := bc.GetTransaction(valSyncOp.InitTxHash())
	if initTx == nil {
		return ErrTxNF
	}
	// check init tx data
	iop, err := operation.DecodeBytes(initTx.Data)
	if err != nil {
		slog.Error("can`t unmarshal validator sync operation from tx data", "err", err)
		return err
	}
	switch initTxData := iop.(type) {
	case operation.Deposit:
		if valSyncOp.OpCode() == operation.DepositCode {
			return ErrInvalidOpCode
		}
		if initTxData.CreatorAddress() != valSyncOp.Creator() {
			return ErrInvalidCreator
		}
	case operation.Exit:
		if valSyncOp.OpCode() == operation.ExitCode {
			return ErrInvalidOpCode
		}
		if initTxData.CreatorAddress() != valSyncOp.Creator() {
			return ErrInvalidCreator
		}
		if initTxData.ExitAfterEpoch() != nil && *initTxData.ExitAfterEpoch() > valSyncOp.ProcEpoch() {
			return ErrInvalidOpEpoch
		}
	case operation.Withdrawal:
		if valSyncOp.OpCode() == operation.WithdrawalCode {
			return ErrInvalidOpCode
		}
		if initTxData.CreatorAddress() != valSyncOp.Creator() {
			return ErrInvalidCreator
		}
		if valSyncOp.Version() == operation.Ver1 && valSyncOp.Balance() == nil {
			return operation.ErrNoBalance
		}
	default:
		return ErrInvalidOpCode
	}

	// check initial tx status
	rc, _, _ := bc.GetTransactionReceipt(valSyncOp.InitTxHash())
	if rc == nil {
		return ErrReceiptNF
	}
	if rc.Status != types.ReceiptStatusSuccessful {
		return ErrInvalidReceiptStatus
	}

	return nil
}

// CompareValSync compares a stored ValidatorSync record with an incoming operation.
func CompareValSync(saved *types.ValidatorSync, input operation.ValidatorSync, isForkDelegate bool) bool {
	if saved.InitTxHash != input.InitTxHash() {
		slog.Warn("check validator sync failed: InitTxHash", "s.InitTxHash", saved.InitTxHash.Hex(), "i.InitTxHash", input.InitTxHash().Hex())
		return false
	}

	if saved.OpType != input.OpType() {
		slog.Warn("check validator sync failed: OpType", "s.OpType", saved.OpType, "i.OpType", input.OpType())
		return false
	}

	if saved.Creator != input.Creator() {
		slog.Warn("check validator sync failed: Creator",
			"s.Creator", fmt.Sprintf("%#x", saved.Creator),
			"i.Creator", fmt.Sprintf("%#x", input.Creator()))
		return false
	}

	if saved.Index != input.Index() {
		slog.Warn("check validator sync failed: Index", "s.Index", saved.Index, "i.Index", input.Index())
		return false
	}

	if !isForkDelegate {
		if saved.ProcEpoch > input.ProcEpoch() {
			slog.Warn("check validator sync failed: ProcEpoch", "s.ProcEpoch", saved.ProcEpoch, "i.ProcEpoch", input.ProcEpoch())
			return false
		}
	}

	if saved.Amount != nil && input.Amount() != nil && saved.Amount.Cmp(input.Amount()) != 0 {
		slog.Warn("check validator sync failed: Amount", "s.Amount", saved.Amount.String(), "i.Amount", input.Amount().String())
		return false
	}

	if saved.Amount != nil && input.Amount() == nil || saved.Amount == nil && input.Amount() != nil {
		slog.Warn("check validator sync failed: Amount nil", "s.Amount", saved.Amount, "i.Amount", input.Amount())
		return false
	}

	return true
}

// ValidatePartialDepositOp validates that a partial deposit op matches stored validator data.
func ValidatePartialDepositOp(validator *valStore.Validator, op operation.Deposit) error {
	//should never happen
	if validator.Address != op.CreatorAddress() {
		return fmt.Errorf("mismatch validator creator address (expect=%#x)", validator.Address)
	}
	if validator.PubKey != op.PubKey() {
		return fmt.Errorf("mismatch validator public key (expect=%#x)", validator.PubKey)
	}
	if validator.WithdrawalAddress != nil && *validator.WithdrawalAddress != op.WithdrawalAddress() {
		return fmt.Errorf("mismatch validator withdrawal address (expect=%#x)", *validator.WithdrawalAddress)
	}
	//check DelegatingStake
	valBin, err := validator.DelegatingStake.MarshalBinary()
	if err != nil {
		return err
	}
	opBin, err := op.DelegatingStake().MarshalBinary()
	if err != nil {
		return err
	}
	if !bytes.Equal(valBin, opBin) {
		return fmt.Errorf("mismatch validator delegating stake rules")
	}
	return nil
}

// ProcessRewards distributes the block reward for coinbase, respecting
// delegating stake profit-share rules where applicable.
func (p *Processor) ProcessRewards(coinbase [20]byte, reward *big.Int) error {
	creatorAddress := common.Address(coinbase)

	if !p.blockchain.Config().IsForkSlotValSyncProc(p.ctx.Slot) {
		p.state.AddBalance(creatorAddress, reward)
		return nil
	}

	val, err := p.storage.GetValidator(p.state, creatorAddress)
	if err != nil {
		return err
	}
	if val == nil {
		return ErrUnknownValidator
	}

	if val.DelegatingStake != nil {
		delegateRules := &val.DelegatingStake.Rules
		isTrial, err := p.IsValidatorTrialPeriod(val)
		if err != nil {
			return err
		}
		if isTrial {
			delegateRules = &val.DelegatingStake.TrialRules
		}
		for address, percent := range delegateRules.ProfitShare() {
			amount := new(big.Int).Mul(reward, big.NewInt(int64(percent)))
			amount.Div(amount, big.NewInt(100))
			p.state.AddBalance(address, amount)
		}
		return nil
	}

	if val.WithdrawalAddress == nil {
		p.state.AddBalance(creatorAddress, reward)
	} else {
		p.state.AddBalance(*val.WithdrawalAddress, reward)
	}
	return nil
}

func (p *Processor) updateValidatorVersionBySlot(validator *valStore.Validator) *valStore.Validator {
	ver := valStore.NoVer
	bcConf := p.blockchain.Config()
	slot := p.ctx.Slot
	if bcConf.IsForkSlotValOpTracking(slot) {
		ver = valStore.Ver1
	}
	validator.SetVersion(ver)
	return validator
}
