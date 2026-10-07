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

/** DelegatingStakeRules */
func TestDelegatingStakeRules_init(t *testing.T) {
	profitShare, stakeShare, exit, withdrawal := TestParamsDelegatingStakeRules()

	dsr := NewDelegatingStakeRules(profitShare, stakeShare, exit, withdrawal)

	assertEqual(t, profitShare, dsr.ProfitShare())
	assertEqual(t, stakeShare, dsr.StakeShare())
	assertEqual(t, exit, dsr.Exit())
	assertEqual(t, withdrawal, dsr.Withdrawal())
}

func TestDelegatingStakeRules_Copy(t *testing.T) {
	profitShare, stakeShare, exit, withdrawal := TestParamsDelegatingStakeRules()

	dsr := NewDelegatingStakeRules(profitShare, stakeShare, exit, withdrawal)

	cpy := dsr.Copy()
	assertEqual(t, dsr.ProfitShare(), cpy.ProfitShare())
	assertEqual(t, dsr.StakeShare(), cpy.StakeShare())
	assertEqual(t, dsr.Exit(), cpy.Exit())
	assertEqual(t, dsr.Withdrawal(), cpy.Withdrawal())

	dsrEmpty := &DelegatingStakeRules{}

	cpyEmpty := dsrEmpty.Copy()
	assertEqual(t, dsrEmpty.ProfitShare(), cpyEmpty.ProfitShare())
	assertEqual(t, dsrEmpty.StakeShare(), cpyEmpty.StakeShare())
	assertEqual(t, dsrEmpty.Exit(), cpyEmpty.Exit())
	assertEqual(t, dsrEmpty.Withdrawal(), cpyEmpty.Withdrawal())
}

func TestDelegatingStakeRules_validate(t *testing.T) {
	profitShare, stakeShare, exit, withdrawal := TestParamsDelegatingStakeRules()

	type decodedOp struct {
		profitShare map[common.Address]uint8
		stakeShare  map[common.Address]uint8
		exit        []common.Address
		withdrawal  []common.Address
	}

	cases := []operationTestCase{
		{
			caseName: "OK",
			decoded: decodedOp{
				profitShare: profitShare,
				stakeShare:  stakeShare,
				exit:        exit,
				withdrawal:  withdrawal,
			},
			encoded: nil,
			errs:    []error{},
		},
		{
			caseName: "ErrBadProfitShare",
			decoded: decodedOp{
				profitShare: nil,
				stakeShare:  stakeShare,
				exit:        exit,
				withdrawal:  withdrawal,
			},
			encoded: nil,
			errs:    []error{ErrBadProfitShare},
		},
		{
			caseName: "ErrBadProfitShare",
			decoded: decodedOp{
				profitShare: map[common.Address]uint8{
					common.HexToAddress("0x1111111111111111111111111111111111111111"): 10,
					common.HexToAddress("0x2222222222222222222222222222222222222222"): 30,
				},
				stakeShare: stakeShare,
				exit:       exit,
				withdrawal: withdrawal,
			},
			encoded: nil,
			errs:    []error{ErrBadProfitShare},
		},
		{
			caseName: "ErrBadStakeShare",
			decoded: decodedOp{
				profitShare: profitShare,
				stakeShare: map[common.Address]uint8{
					common.HexToAddress("0x1111111111111111111111111111111111111111"): 60,
					common.HexToAddress("0x2222222222222222222222222222222222222222"): 60,
				},
				exit:       exit,
				withdrawal: withdrawal,
			},
			encoded: nil,
			errs:    []error{ErrBadStakeShare},
		},
		{
			caseName: "ErrNoExitRoles",
			decoded: decodedOp{
				profitShare: profitShare,
				stakeShare:  stakeShare,
				exit:        nil,
				withdrawal:  withdrawal,
			},
			encoded: nil,
			errs:    []error{ErrNoExitRoles},
		},
		{
			caseName: "ErrNoWithdrawalRoles",
			decoded: decodedOp{
				profitShare: profitShare,
				stakeShare:  stakeShare,
				exit:        exit,
				withdrawal:  nil,
			},
			encoded: nil,
			errs:    []error{ErrNoWithdrawalRoles},
		},
	}

	operationEncode := func(b []byte, i interface{}) error {
		o := i.(decodedOp)
		createOp := NewDelegatingStakeRules(
			o.profitShare,
			o.stakeShare,
			o.exit,
			o.withdrawal,
		)

		return createOp.Validate()
	}

	operationDecode := func(b []byte, i interface{}) error {
		return nil
	}

	startSubTests(t, cases, operationEncode, operationDecode)
}

func TestDelegatingStakeRules_Marshaling(t *testing.T) {
	defer func(tStart time.Time) {
		fmt.Println("TOTAL TIME", "elapsed", time.Since(tStart))
	}(time.Now())
	profitShare, stakeShare, exit, withdrawal := TestParamsDelegatingStakeRules()
	encoded := hexToBytes("f8a9f893941111111111111111111111111111111111111111" +
		"942222222222222222222222222222222222222222" +
		"943333333333333333333333333333333333333333" +
		"944444444444444444444444444444444444444444" +
		"945555555555555555555555555555555555555555" +
		"946666666666666666666666666666666666666666" +
		"947777777777777777777777777777777777777777" +
		"870a1e3c0000000087000000461e000081a081c0")

	dsr := NewDelegatingStakeRules(profitShare, stakeShare, exit, withdrawal)

	bin, err := dsr.MarshalBinary()
	assertNoError(t, err)

	fmt.Println(fmt.Sprintf("binary_size=%d", len(bin)))
	assertEqual(t, encoded, bin)

	unmarshaled := &DelegatingStakeRules{}
	err = unmarshaled.UnmarshalBinary(bin)
	assertNoError(t, err)

	assertEqual(t, dsr.ProfitShare(), unmarshaled.ProfitShare())
	assertEqual(t, dsr.StakeShare(), unmarshaled.StakeShare())
	assertEqual(t, dsr.Exit(), unmarshaled.Exit())
	assertEqual(t, dsr.Withdrawal(), unmarshaled.Withdrawal())
}
