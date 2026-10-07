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
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/LFDT-Iguazu/iguazu-types/common"
)

type operationTestCase struct {
	caseName string
	decoded  interface{}
	encoded  []byte
	errs     []error
}

func checkOpCode(b []byte, op Operation) error {
	haveOpCode, err := GetOpCode(b)
	if err != nil {
		return err
	}
	if haveOpCode != op.OpCode() {
		return fmt.Errorf("values do not match:\nwant: %+v\nhave: %+v", op.OpCode(), haveOpCode)
	}
	return nil
}

func equalOpBytes(op Operation, b []byte) error {
	have, err := EncodeToBytes(op)
	if err != nil {
		return fmt.Errorf("can`t encode operation %+v\nerror: %+v", op, err)
	}

	if !bytes.Equal(b, have) {
		return fmt.Errorf("values do not match:\n want: %#x\nhave: %#x", b, have)
	}

	return nil
}

func startSubTests(t *testing.T, cases []operationTestCase, operationEncode, operationDecode func([]byte, interface{}) error) {
	t.Helper()

	for _, c := range cases {
		var err error
		t.Run("encoding"+" "+c.caseName, func(t *testing.T) {
			err = operationEncode(c.encoded, c.decoded)
			if !checkError(err, c.errs) {
				t.Errorf("operationEncode: invalid test case %s\nwant errors: %s\nhave errors: %s", c.caseName, c.errs, err)
			}
		})
		if err != nil {
			continue
		}
		t.Run("decoding"+" "+c.caseName, func(t *testing.T) {
			err = operationDecode(c.encoded, c.decoded)
			if !checkError(err, c.errs) {
				t.Errorf("operationDecode: invalid test case %s\nwant errors: %s\nhave errors: %s", c.caseName, c.errs, err)
			}
		})
	}
}

func TestParamsDelegatingStakeRules() (
	profitShare, stakeShare map[common.Address]uint8,
	exit, withdrawal []common.Address,
) {
	profitShare = map[common.Address]uint8{
		common.HexToAddress("0x1111111111111111111111111111111111111111"): 10,
		common.HexToAddress("0x2222222222222222222222222222222222222222"): 30,
		common.HexToAddress("0x3333333333333333333333333333333333333333"): 60,
	}
	stakeShare = map[common.Address]uint8{
		common.HexToAddress("0x4444444444444444444444444444444444444444"): 70,
		common.HexToAddress("0x5555555555555555555555555555555555555555"): 30,
	}
	exit = []common.Address{common.HexToAddress("0x6666666666666666666666666666666666666666")}
	withdrawal = []common.Address{common.HexToAddress("0x7777777777777777777777777777777777777777")}
	return
}

// checkError reports whether err matches any of the expected errors.
func checkError(e error, arr []error) bool {
	if e == nil && len(arr) == 0 {
		return true
	}
	for _, err := range arr {
		if errors.Is(e, err) {
			return true
		}
	}
	return false
}

// bigIntEquals reports whether have == want, treating nil as zero.
func bigIntEquals(have, want *big.Int) bool {
	if have == nil && want == nil {
		return true
	}
	if want == nil {
		want = big.NewInt(0)
	}
	return have.Cmp(want) == 0
}

// assertEqual fails the test if exp and act are not deeply equal.
func assertEqual(t *testing.T, exp, act interface{}) {
	t.Helper()
	if !reflect.DeepEqual(exp, act) {
		t.Fatalf("\n\tExpect:\t%v\n\tGot:\t%v", exp, act)
	}
}

// assertNoError fails the test if err is non-nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("No error expected, but got: %s", err)
	}
}

// assertError fails the test if err does not match expErr.
func assertError(t *testing.T, err, expErr error) {
	t.Helper()
	if !errors.Is(err, expErr) {
		t.Fatalf("\n\tExpect error:\t%s\n\tGot:\t%s", expErr, err)
	}
}

// hexToBytes decodes a hex string (with or without 0x prefix) to bytes.
// Panics on invalid input (test helper).
func hexToBytes(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return []byte{}
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("hexToBytes: invalid hex string %q: %v", s, err))
	}
	return b
}
