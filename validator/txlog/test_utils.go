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

package txlog

import (
	"math/big"
	"reflect"
	"testing"
)

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

// assertNil fails the test if v is not nil.
func assertNil(t *testing.T, v interface{}) {
	t.Helper()
	if v != nil && !reflect.ValueOf(v).IsNil() {
		t.Fatalf("Expected nil, but got: %v", v)
	}
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

// ptrUint64 returns a pointer to the given uint64 value.
func ptrUint64(v uint64) *uint64 {
	return &v
}
