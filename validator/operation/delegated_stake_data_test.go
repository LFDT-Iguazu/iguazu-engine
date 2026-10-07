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

package operation

import (
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Iguazu/iguazu-types/common"
)

func TestDelegatingStakeData_NewDelegatingStakeData(t *testing.T) {
	profitShare, stakeShare, exit, withdrawal := TestParamsDelegatingStakeRules()
	trialPeriod := uint64(321)

	rules := NewDelegatingStakeRules(profitShare, stakeShare, exit, withdrawal)
	trialRules := NewDelegatingStakeRules(profitShare, stakeShare, exit, withdrawal)

	dsr, err := NewDelegatingStakeData(rules, trialPeriod, trialRules)
	assertNoError(t, err)
	assertEqual(t, profitShare, dsr.Rules.ProfitShare())
	assertEqual(t, stakeShare, dsr.Rules.StakeShare())
	assertEqual(t, exit, dsr.Rules.Exit())
	assertEqual(t, withdrawal, dsr.Rules.Withdrawal())
	assertEqual(t, trialPeriod, dsr.TrialPeriod)
	assertEqual(t, profitShare, dsr.TrialRules.ProfitShare())
	assertEqual(t, stakeShare, dsr.TrialRules.StakeShare())
	assertEqual(t, exit, dsr.TrialRules.Exit())
	assertEqual(t, withdrawal, dsr.TrialRules.Withdrawal())

	// empty instance
	dsrEmpty, err := NewDelegatingStakeData(nil, 0, nil)
	assertNoError(t, err)
	assertEqual(t, map[common.Address]uint8{}, dsrEmpty.Rules.ProfitShare())
	assertEqual(t, map[common.Address]uint8{}, dsrEmpty.Rules.StakeShare())
	assertEqual(t, []common.Address{}, dsrEmpty.Rules.Exit())
	assertEqual(t, []common.Address{}, dsrEmpty.Rules.Withdrawal())
	assertEqual(t, uint64(0), dsrEmpty.TrialPeriod)
	assertEqual(t, map[common.Address]uint8{}, dsrEmpty.TrialRules.ProfitShare())
	assertEqual(t, map[common.Address]uint8{}, dsrEmpty.TrialRules.StakeShare())
	assertEqual(t, []common.Address{}, dsrEmpty.TrialRules.Exit())
	assertEqual(t, []common.Address{}, dsrEmpty.TrialRules.Withdrawal())
}

func TestDelegatingStakeData_Marshaling(t *testing.T) {
	defer func(tStart time.Time) {
		fmt.Println("TOTAL TIME", "elapsed", time.Since(tStart))
	}(time.Now())

	profitShare, stakeShare, exit, withdrawal := TestParamsDelegatingStakeRules()
	trialPeriod := uint64(321)

	rules := NewDelegatingStakeRules(profitShare, stakeShare, exit, withdrawal)
	trialRules := NewDelegatingStakeRules(profitShare, stakeShare, exit, withdrawal)

	dsr, err := NewDelegatingStakeData(rules, trialPeriod, trialRules)
	assertNoError(t, err)

	bin, err := dsr.MarshalBinary()
	assertNoError(t, err)

	unmarshaled := &DelegatingStakeData{}
	err = unmarshaled.UnmarshalBinary(bin)
	assertNoError(t, err)

	assertEqual(t, dsr.Rules, unmarshaled.Rules)
	assertEqual(t, dsr.TrialPeriod, unmarshaled.TrialPeriod)
	assertEqual(t, dsr.TrialRules, unmarshaled.TrialRules)

	//// empty instance
	dsrEmpty, err := NewDelegatingStakeData(nil, 0, nil)
	assertNoError(t, err)

	binEmpty, err := dsrEmpty.MarshalBinary()
	assertNoError(t, err)

	unmarshaledEmpty := &DelegatingStakeData{}
	err = unmarshaled.UnmarshalBinary(binEmpty)
	assertNoError(t, err)

	assertEqual(t, dsrEmpty.Rules, unmarshaledEmpty.Rules)
	assertEqual(t, dsrEmpty.TrialPeriod, unmarshaledEmpty.TrialPeriod)
	assertEqual(t, dsrEmpty.TrialRules, unmarshaledEmpty.TrialRules)

	// nil instance
	var dsrNil *DelegatingStakeData
	binNil, err := dsrNil.MarshalBinary()
	assertNoError(t, err)

	unmarshaledNil := &DelegatingStakeData{}
	err = unmarshaledNil.UnmarshalBinary(binNil)
	assertError(t, err, errDelegatingStakeNilValBin)
}

func TestDelegatingStakeData_NewDelegatingStakeDataFromBinary(t *testing.T) {
	profitShare, stakeShare, exit, withdrawal := TestParamsDelegatingStakeRules()
	trialPeriod := uint64(321)

	rules := NewDelegatingStakeRules(profitShare, stakeShare, exit, withdrawal)
	trialRules := NewDelegatingStakeRules(profitShare, stakeShare, exit, withdrawal)

	dsr, err := NewDelegatingStakeData(rules, trialPeriod, trialRules)
	assertNoError(t, err)

	bin, err := dsr.MarshalBinary()
	assertNoError(t, err)

	dsrBin, err := NewDelegatingStakeDataFromBinary(bin)
	assertNoError(t, err)

	assertEqual(t, dsr.Rules, dsrBin.Rules)
	assertEqual(t, dsr.TrialPeriod, dsrBin.TrialPeriod)
	assertEqual(t, dsr.TrialRules, dsrBin.TrialRules)

	//// empty instance
	dsrEmpty, err := NewDelegatingStakeData(nil, 0, nil)
	assertNoError(t, err)

	binEmpty, err := dsrEmpty.MarshalBinary()
	assertNoError(t, err)

	dsrBinEmpty, err := NewDelegatingStakeDataFromBinary(binEmpty)
	assertNoError(t, err)

	assertEqual(t, fmt.Sprintf("%#X", dsrEmpty.Rules), fmt.Sprintf("%#X", dsrBinEmpty.Rules))
	assertEqual(t, dsrEmpty.TrialPeriod, dsrBinEmpty.TrialPeriod)
	assertEqual(t, fmt.Sprintf("%#X", dsrEmpty.TrialRules), fmt.Sprintf("%#X", dsrBinEmpty.TrialRules))

	// nil instance
	var dsrNil *DelegatingStakeData
	binNil, err := dsrNil.MarshalBinary()
	assertNoError(t, err)

	dsrBinNil, err := NewDelegatingStakeDataFromBinary(binNil)
	assertNoError(t, err)
	assertEqual(t, dsrNil, dsrBinNil)
}
