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

package validator

import (
	"errors"
	"maps"
	"math"
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types/era"
	"github.com/LFDT-Iguazu/iguazu-types/common"

	"github.com/LFDT-Iguazu/iguazu-engine/validator/operation"
	valStore "github.com/LFDT-Iguazu/iguazu-engine/validator/storage"
)

// ---- test state ----

// testStateDB is an in-memory iface.StateDB with snapshots,
// so a failed call does not leave partial state changes.
type testStateDB struct {
	balances  map[[20]byte]*big.Int
	nonces    map[[20]byte]uint64
	storage   map[[20]byte]map[[32]byte][32]byte
	code      map[[20]byte][]byte
	snapshots []*testStateDB
}

func newTestStateDB() *testStateDB {
	return &testStateDB{
		balances: make(map[[20]byte]*big.Int),
		nonces:   make(map[[20]byte]uint64),
		storage:  make(map[[20]byte]map[[32]byte][32]byte),
		code:     make(map[[20]byte][]byte),
	}
}

func (s *testStateDB) copyData() *testStateDB {
	cpy := newTestStateDB()
	for a, b := range s.balances {
		cpy.balances[a] = new(big.Int).Set(b)
	}
	maps.Copy(cpy.nonces, s.nonces)
	for a, slots := range s.storage {
		cpy.storage[a] = maps.Clone(slots)
	}
	for a, c := range s.code {
		cpy.code[a] = slices.Clone(c)
	}
	return cpy
}

func (s *testStateDB) GetBalance(addr [20]byte) *big.Int {
	if b, ok := s.balances[addr]; ok {
		return new(big.Int).Set(b)
	}
	return new(big.Int)
}
func (s *testStateDB) SetBalance(addr [20]byte, amount *big.Int) {
	s.balances[addr] = new(big.Int).Set(amount)
}
func (s *testStateDB) AddBalance(addr [20]byte, amount *big.Int) {
	s.balances[addr] = new(big.Int).Add(s.GetBalance(addr), amount)
}
func (s *testStateDB) SubBalance(addr [20]byte, amount *big.Int) {
	s.balances[addr] = new(big.Int).Sub(s.GetBalance(addr), amount)
}
func (s *testStateDB) IsValidatorAddress([20]byte) bool { return false }
func (s *testStateDB) GetState(addr [20]byte, key [32]byte) [32]byte {
	return s.storage[addr][key]
}
func (s *testStateDB) SetState(addr [20]byte, key [32]byte, value [32]byte) {
	if s.storage[addr] == nil {
		s.storage[addr] = make(map[[32]byte][32]byte)
	}
	s.storage[addr][key] = value
}
func (s *testStateDB) GetCode(addr [20]byte) []byte         { return s.code[addr] }
func (s *testStateDB) SetCode(addr [20]byte, code []byte)   { s.code[addr] = slices.Clone(code) }
func (s *testStateDB) GetNonce(addr [20]byte) uint64        { return s.nonces[addr] }
func (s *testStateDB) SetNonce(addr [20]byte, nonce uint64) { s.nonces[addr] = nonce }
func (s *testStateDB) Exist([20]byte) bool                  { return true }
func (s *testStateDB) CreateAccount([20]byte)               {}
func (s *testStateDB) Suicide([20]byte) bool                { return false }
func (s *testStateDB) AddLog(*iface.LogEntry)               {}
func (s *testStateDB) Snapshot() int {
	s.snapshots = append(s.snapshots, s.copyData())
	return len(s.snapshots) - 1
}
func (s *testStateDB) RevertToSnapshot(id int) {
	snap := s.snapshots[id]
	s.balances, s.nonces, s.storage, s.code = snap.balances, snap.nonces, snap.storage, snap.code
	s.snapshots = s.snapshots[:id]
}
func (s *testStateDB) ForEachStorage([20]byte, func(key, value [32]byte) bool) error { return nil }

// ---- test blockchain ----

// testChain implements the processor's blockchain interface for UpdateBalance ops.
type testChain struct {
	config   *iface.ChainConfig
	slotInfo *types.SlotInfo
	eraInfo  *era.EraInfo
	storage  valStore.Storage
	initTx   *types.Transaction
	sender   common.Address
	saved    *types.ValidatorSync // validator sync data received from coordinator
}

func (c *testChain) Config() *iface.ChainConfig                    { return c.config }
func (c *testChain) GetSlotInfo() *types.SlotInfo                  { return c.slotInfo }
func (c *testChain) GetEraInfo() *era.EraInfo                      { return c.eraInfo }
func (c *testChain) EpochToEra(uint64) *era.Era                    { return c.eraInfo.GetEra() }
func (c *testChain) ReadEra(uint64) *era.Era                       { return c.eraInfo.GetEra() }
func (c *testChain) GetHeaderByHash(common.Hash) *types.HeaderInfo { return nil }
func (c *testChain) StateAt(common.Hash) (iface.StateDB, error) {
	return nil, errors.New("not supported")
}
func (c *testChain) ValidatorStorage() valStore.Storage { return c.storage }

func (c *testChain) GetValidatorSyncData(initTxHash common.Hash) *types.ValidatorSync {
	if c.saved == nil || c.saved.InitTxHash != initTxHash {
		return nil
	}
	return c.saved.Copy()
}
func (c *testChain) GetNotProcessedValidatorSyncData() map[common.Hash]*types.ValidatorSync {
	return nil
}
func (c *testChain) SetValidatorSyncData(*types.ValidatorSync) {}

func (c *testChain) GetTransaction(txHash common.Hash) (*types.Transaction, common.Hash, uint64) {
	if txHash != initTxHash {
		return nil, common.Hash{}, 0
	}
	return c.initTx, common.Hash{}, 0
}
func (c *testChain) GetTransactionReceipt(txHash common.Hash) (*types.Receipt, common.Hash, uint64) {
	if txHash != initTxHash {
		return nil, common.Hash{}, 0
	}
	return &types.Receipt{Status: types.ReceiptStatusSuccessful}, common.Hash{}, 0
}
func (c *testChain) GetTransactionSender(common.Hash) (common.Address, error) {
	return c.sender, nil
}
func (c *testChain) FixValidatorSyncOpProcessing(iface.ProcessorCtx, iface.ValidatorSyncOpFix, common.Hash, common.Address, common.Address) (bool, []byte, error) {
	return false, nil, nil
}

// ---- test environment ----

var (
	initTxHash        = common.HexToHash("0303030303030303030303030303030303030303030303030303030303030303")
	validatorAddr     = common.HexToAddress("0x6666666666666666666666666666666666666666")
	withdrawalAddress = common.HexToAddress("0x7777777777777777777777777777777777777777")
	depositorAddr     = common.HexToAddress("0x8888888888888888888888888888888888888888")
	pubKey            = common.BlsPubKey{0x01, 0x02, 0x03}
)

// newTestChainConfig mirrors the validator test config of the base implementation:
// ForkSlotValSyncProc=10000 (epoch 312), ValidatorOpExpireSlots=100.
func newTestChainConfig() *iface.ChainConfig {
	return &iface.ChainConfig{
		ChainID:                111111,
		EpochsPerEra:           22,
		SlotsPerEpoch:          32,
		ValidatorsPerSlot:      2,
		TransitionPeriod:       2,
		ValidatorsStateAddress: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		EffectiveBalance:       big.NewInt(3200),
		ValidatorOpExpireSlots: 100,
		ForkSlotDelegate:       10,
		ForkSlotValSyncProc:    10000,
		ForkSlotValOpTracking:  100,
	}
}

// valSyncAttackEnv prepares a processor to apply an UpdateBalance ValSync tx
// after ForkSlotValSyncProc, where saved (coordinator) data and tx data may differ.
type valSyncAttackEnv struct {
	processor *Processor
	state     *testStateDB
	bc        *testChain
	procSlot  uint64
	procEpoch uint64
	procEra   uint64
}

func newValSyncAttackEnv(t *testing.T) *valSyncAttackEnv {
	t.Helper()

	withdrawalOp, err := operation.NewWithdrawalOperation(validatorAddr, new(big.Int))
	requireNoError(t, err)
	initTxData, err := operation.EncodeToBytes(withdrawalOp)
	requireNoError(t, err)

	conf := newTestChainConfig()
	bc := &testChain{
		config: conf,
		slotInfo: &types.SlotInfo{
			GenesisTime:    uint64(time.Now().Unix()),
			SecondsPerSlot: 4,
			SlotsPerEpoch:  conf.SlotsPerEpoch,
		},
		eraInfo: era.NewEraInfo(&era.Era{Number: 7, From: 89, To: 110}),
		storage: valStore.NewStorage(&valStore.ChainConfig{
			SlotsPerEpoch:          conf.SlotsPerEpoch,
			ValidatorsStateAddress: &conf.ValidatorsStateAddress,
			ValidatorsPerSlot:      conf.ValidatorsPerSlot,
		}),
		initTx: &types.Transaction{Data: initTxData},
		sender: depositorAddr,
	}

	env := &valSyncAttackEnv{
		state:    newTestStateDB(),
		bc:       bc,
		procSlot: conf.ForkSlotValSyncProc,
	}
	env.procEpoch = env.procSlot / conf.SlotsPerEpoch
	env.procEra = env.procEpoch / conf.EpochsPerEra
	env.processor = NewProcessor(iface.BlockContext{
		BlockNumber: big.NewInt(8000000),
		Time:        big.NewInt(1658844221),
		Slot:        env.procSlot,
		Era:         env.procEra,
	}, env.state, bc)
	return env
}

type accountRef common.Address

func (a accountRef) Address() common.Address { return common.Address(a) }

type testMessage struct {
	data []byte
}

func (m *testMessage) TxHash() common.Hash { return common.Hash{} }
func (m *testMessage) Data() []byte        { return m.data }

// call applies the op and checks the result against the expected error.
func (env *valSyncAttackEnv) call(t *testing.T, op operation.Operation, wantErr error) {
	t.Helper()
	data, err := operation.EncodeToBytes(op)
	requireNoError(t, err)
	_, err = env.processor.Call(accountRef(withdrawalAddress), env.processor.GetValidatorsStateAddress(), MinDepositVal, &testMessage{data: data})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Call error = %v, want %v", err, wantErr)
	}
}

func (env *valSyncAttackEnv) setValidator(t *testing.T, v *valStore.Validator) {
	t.Helper()
	requireNoError(t, env.processor.Storage().SetValidator(env.state, v))
}

func (env *valSyncAttackEnv) getValidator(t *testing.T) *valStore.Validator {
	t.Helper()
	v, err := env.processor.Storage().GetValidator(env.state, validatorAddr)
	requireNoError(t, err)
	return v
}

// apply sets the coordinator's saved data (savedActEpoch, savedExitEpoch) and applies
// a tx built by the block creator with arbitrary (txActEpoch, txExitEpoch).
func (env *valSyncAttackEnv) apply(t *testing.T, savedActEpoch, savedExitEpoch, txActEpoch, txExitEpoch uint64) {
	t.Helper()

	amount, _ := new(big.Int).SetString("200000000000000000000", 10)
	balance, _ := new(big.Int).SetString("3100000000000000000000", 10)

	op, err := operation.NewValidatorSyncOperation(
		operation.Ver2, types.UpdateBalance, initTxHash, env.procEpoch, 0, validatorAddr,
		amount, &withdrawalAddress, balance, txActEpoch, txExitEpoch, new(big.Int),
	)
	requireNoError(t, err)

	// data received from coordinator
	env.bc.saved = &types.ValidatorSync{
		InitTxHash:      initTxHash,
		OpType:          types.UpdateBalance,
		ProcEpoch:       env.procEpoch,
		Creator:         validatorAddr,
		Amount:          amount,
		Balance:         balance,
		ActivationEpoch: savedActEpoch,
		ExitEpoch:       savedExitEpoch,
		StakeAmount:     new(big.Int),
	}
	env.call(t, op, nil)
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

const ffEpoch = uint64(math.MaxUint64)

// epochs of activation/exit
type actExitEpochs struct {
	act, exit uint64
}

// TestValSyncFix_Activation checks the fix of lost activation while UpdateBalance:
// the validator is not activated; epochs are taken from coordinator data only.
// Test config: ForkSlotValSyncProc=10000 (epoch 312), ValidatorOpExpireSlots=100,
// so the lost op is expired if its epoch <= 309.
func TestValSyncFix_Activation(t *testing.T) {
	cases := []struct {
		name      string
		saved     actExitEpochs // coordinator data
		tx        actExitEpochs // set by block creator
		activated bool
	}{
		{name: "lost activation", saved: actExitEpochs{100, ffEpoch}, tx: actExitEpochs{100, ffEpoch}, activated: true},
		{name: "forged tx epochs are ignored: activation", saved: actExitEpochs{100, ffEpoch}, tx: actExitEpochs{ffEpoch, ffEpoch}, activated: true},
		{name: "forged tx epochs are ignored: not activated", saved: actExitEpochs{ffEpoch, ffEpoch}, tx: actExitEpochs{1, ffEpoch}},
		{name: "activated and exited", saved: actExitEpochs{100, 200}, tx: actExitEpochs{100, 200}},
		{name: "not expired", saved: actExitEpochs{310, ffEpoch}, tx: actExitEpochs{310, ffEpoch}},
		{name: "future epoch", saved: actExitEpochs{400, ffEpoch}, tx: actExitEpochs{400, ffEpoch}},
		{name: "zero epoch", saved: actExitEpochs{0, ffEpoch}, tx: actExitEpochs{0, ffEpoch}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newValSyncAttackEnv(t)

			v := valStore.NewValidator(pubKey, validatorAddr, &withdrawalAddress)
			v.SetVersion(valStore.Ver1)
			// insufficient deposit of the init tx sender
			stake, _ := new(big.Int).SetString("300000000000000000000", 10)
			v.AddStake(depositorAddr, stake)
			v.SetWithdrawalTx(&initTxHash) // pending refund
			env.setValidator(t, v)

			env.apply(t, c.saved.act, c.saved.exit, c.tx.act, c.tx.exit)

			val := env.getValidator(t)
			inList := slices.Contains(env.processor.Storage().GetValidatorsList(env.state), validatorAddr)
			if c.activated {
				if val.ActivationEra != env.procEra+postpone || !inList {
					t.Fatalf("activation era = %d, in list = %v; want %d, true", val.ActivationEra, inList, env.procEra+postpone)
				}
			} else if val.ActivationEra != ffEpoch || inList {
				t.Fatalf("activation era = %d, in list = %v; want not activated", val.ActivationEra, inList)
			}
		})
	}
}

// TestValSyncFix_Deactivation checks the fix of lost deactivation while UpdateBalance:
// epochs are taken from coordinator data only.
func TestValSyncFix_Deactivation(t *testing.T) {
	cases := []struct {
		name     string
		actEra   int64 // activation era relative to the current one
		exitEra  uint64
		saved    actExitEpochs // coordinator data
		tx       actExitEpochs // set by block creator
		expExits bool
	}{
		{name: "lost deactivation", actEra: -1, exitEra: ffEpoch, saved: actExitEpochs{100, 200}, tx: actExitEpochs{100, 200}, expExits: true},
		{name: "forged tx epochs are ignored: deactivation", actEra: -1, exitEra: ffEpoch, saved: actExitEpochs{100, 200}, tx: actExitEpochs{100, ffEpoch}, expExits: true},
		{name: "forged tx epochs are ignored: active", actEra: -1, exitEra: ffEpoch, saved: actExitEpochs{100, ffEpoch}, tx: actExitEpochs{100, 1}},
		{name: "not expired", actEra: -1, exitEra: ffEpoch, saved: actExitEpochs{100, 310}, tx: actExitEpochs{100, 310}},
		{name: "future epoch", actEra: -1, exitEra: ffEpoch, saved: actExitEpochs{100, 400}, tx: actExitEpochs{100, 400}},
		{name: "zero epoch", actEra: -1, exitEra: ffEpoch, saved: actExitEpochs{100, 0}, tx: actExitEpochs{100, 0}},
		{name: "not activated by coordinator", actEra: -1, exitEra: ffEpoch, saved: actExitEpochs{ffEpoch, 200}, tx: actExitEpochs{ffEpoch, 200}},
		{name: "activation era is not reached", actEra: 0, exitEra: ffEpoch, saved: actExitEpochs{100, 200}, tx: actExitEpochs{100, 200}},
		{name: "already exited", actEra: -1, exitEra: 10, saved: actExitEpochs{100, 200}, tx: actExitEpochs{100, 200}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newValSyncAttackEnv(t)

			v := valStore.NewValidator(pubKey, validatorAddr, &withdrawalAddress)
			v.SetVersion(valStore.Ver1)
			v.SetActivationEra(uint64(int64(env.procEra) + c.actEra))
			v.SetExitEra(c.exitEra)
			v.SetWithdrawalTx(&initTxHash) // pending withdrawal
			env.setValidator(t, v)

			env.apply(t, c.saved.act, c.saved.exit, c.tx.act, c.tx.exit)

			want := c.exitEra
			if c.expExits {
				want = env.procEra + postpone
			}
			if got := env.getValidator(t).ExitEra; got != want {
				t.Fatalf("exit era = %d, want %d", got, want)
			}
		})
	}
}

type delegateUpdateBalanceCase struct {
	version      operation.VersionValSyncOp
	amount       *big.Int
	txBalance    *big.Int
	txStake      *big.Int // stake amount set by block creator
	savedBalance *big.Int
	savedStake   *big.Int // stake amount received from coordinator
	wantErr      error
	beforeFork   bool // apply before ForkSlotValSyncProc
}

// runDelegateUpdateBalance applies an UpdateBalance tx of a delegating validator
// and returns the amounts paid out to profit and stake share addresses.
func runDelegateUpdateBalance(t *testing.T, c delegateUpdateBalanceCase) (profitPaid, stakePaid *big.Int) {
	t.Helper()

	dsProfitShare, dsStakeShare, dsExit, dsWithdrawal := operation.TestParamsDelegatingStakeRules()
	rules := operation.NewDelegatingStakeRules(dsProfitShare, dsStakeShare, dsExit, dsWithdrawal)
	delegateData, err := operation.NewDelegatingStakeData(rules, 2, rules)
	requireNoError(t, err)

	env := newValSyncAttackEnv(t)
	if c.beforeFork {
		env.processor.ctx.Slot = env.procSlot - 1
	}

	v := valStore.NewValidator(pubKey, validatorAddr, &withdrawalAddress)
	v.SetVersion(valStore.Ver1)
	v.DelegatingStake = delegateData
	v.SetActivationEra(env.procEra + 1) // activated, trial period
	v.SetWithdrawalTx(&initTxHash)
	env.setValidator(t, v)

	op, err := operation.NewValidatorSyncOperation(
		c.version, types.UpdateBalance, initTxHash, env.procEpoch, 0, validatorAddr,
		c.amount, &withdrawalAddress, c.txBalance, 0, 0, c.txStake,
	)
	requireNoError(t, err)
	env.bc.saved = &types.ValidatorSync{
		InitTxHash:  initTxHash,
		OpType:      types.UpdateBalance,
		ProcEpoch:   env.procEpoch,
		Creator:     validatorAddr,
		Amount:      c.amount,
		Balance:     c.savedBalance,
		StakeAmount: c.savedStake,
	}

	total := func(shares map[common.Address]uint8) *big.Int {
		sum := new(big.Int)
		for a := range shares {
			sum.Add(sum, env.state.GetBalance(a))
		}
		return sum
	}
	profitBefore, stakeBefore := total(rules.ProfitShare()), total(rules.StakeShare())
	env.call(t, op, c.wantErr)
	return new(big.Int).Sub(total(rules.ProfitShare()), profitBefore), new(big.Int).Sub(total(rules.StakeShare()), stakeBefore)
}

func wat(v int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(v), common.BigWat)
}

func requireNothingPaid(t *testing.T, profitPaid, stakePaid *big.Int) {
	t.Helper()
	if profitPaid.Sign() != 0 || stakePaid.Sign() != 0 {
		t.Fatalf("paid rewards=%s stake=%s, want nothing", profitPaid, stakePaid)
	}
}

// TestValSyncAttack_ForgedBalance checks that a forged Balance does not affect the payout.
// Op ver2 has no balance at all (the passed value is ignored).
func TestValSyncAttack_ForgedBalance(t *testing.T) {
	amount := wat(200)
	profitPaid, stakePaid := runDelegateUpdateBalance(t, delegateUpdateBalanceCase{
		version:      operation.Ver2,
		amount:       amount,
		txBalance:    wat(1000000), // forged
		txStake:      wat(0),
		savedBalance: wat(3200),
		savedStake:   wat(0),
	})
	if paid := new(big.Int).Add(profitPaid, stakePaid); paid.Cmp(amount) != 0 {
		t.Errorf("forged Balance paid out %s wei, withdrawal amount is %s wei", paid, amount)
	}
}

// TestValSyncAttack_ForgedStakeAmount checks that the block creator is not able
// to change the split between stake and rewards.
func TestValSyncAttack_ForgedStakeAmount(t *testing.T) {
	cases := []struct {
		name       string
		txStake    *big.Int
		savedStake *big.Int
	}{
		{name: "stake shifted to rewards", txStake: wat(0), savedStake: wat(400)},
		{name: "rewards shifted to stake", txStake: wat(500), savedStake: wat(400)},
		{name: "no coordinator stake amount", txStake: wat(0), savedStake: nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			profitPaid, stakePaid := runDelegateUpdateBalance(t, delegateUpdateBalanceCase{
				version:      operation.Ver2,
				amount:       wat(500),
				txBalance:    wat(2800),
				txStake:      c.txStake,
				savedBalance: wat(2800),
				savedStake:   c.savedStake,
				wantErr:      ErrMismatchValSyncOp,
			})
			requireNothingPaid(t, profitPaid, stakePaid)
		})
	}
}

// TestValSyncAttack_OldOpVersion checks that after the fork the op without stake amount (Ver1) is rejected.
func TestValSyncAttack_OldOpVersion(t *testing.T) {
	profitPaid, stakePaid := runDelegateUpdateBalance(t, delegateUpdateBalanceCase{
		version:      operation.Ver1,
		amount:       wat(500),
		txBalance:    wat(2800),
		savedBalance: wat(2800),
		savedStake:   wat(400),
		wantErr:      ErrInvalidOpVersion,
	})
	requireNothingPaid(t, profitPaid, stakePaid)
}

// TestValSyncAttack_Ver2BeforeFork checks that op ver2 (without balance)
// applied before the fork fails instead of panic in the legacy payout formula.
func TestValSyncAttack_Ver2BeforeFork(t *testing.T) {
	profitPaid, stakePaid := runDelegateUpdateBalance(t, delegateUpdateBalanceCase{
		version:      operation.Ver2,
		amount:       wat(500),
		txStake:      wat(400),
		savedBalance: wat(2800),
		savedStake:   wat(400),
		wantErr:      operation.ErrNoBalance,
		beforeFork:   true,
	})
	requireNothingPaid(t, profitPaid, stakePaid)
}

// TestDelegateRules_DeactivatedWithdrawalSplit checks payouts of a deactivated validator
// with coordinator data: the stake amount is calculated by coordinator (rewards are withdrawn first).
func TestDelegateRules_DeactivatedWithdrawalSplit(t *testing.T) {
	// EffectiveBalance of test config = 3200 wat; balance before withdrawal = 3300 wat
	cases := []struct {
		name    string
		amount  *big.Int
		stake   *big.Int
		balance *big.Int // after debit
	}{
		{name: "full withdrawal (op.Amount=0 on coordinator)", amount: wat(3300), stake: wat(3200), balance: wat(0)},
		{name: "partial: rewards+stake", amount: wat(500), stake: wat(400), balance: wat(2800)},
		{name: "partial: part of rewards", amount: wat(20), stake: wat(0), balance: wat(3280)},
		{name: "partial: part of rewards (larger)", amount: wat(90), stake: wat(0), balance: wat(3210)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			profitPaid, stakePaid := runDelegateUpdateBalance(t, delegateUpdateBalanceCase{
				version:      operation.Ver2,
				amount:       c.amount,
				txBalance:    c.balance,
				txStake:      c.stake,
				savedBalance: c.balance,
				savedStake:   c.stake,
			})
			expProfit := new(big.Int).Sub(c.amount, c.stake)
			if profitPaid.Cmp(expProfit) != 0 || stakePaid.Cmp(c.stake) != 0 {
				t.Errorf("paid rewards=%s stake=%s, expected rewards=%s stake=%s", profitPaid, stakePaid, expProfit, c.stake)
			}
		})
	}
}
