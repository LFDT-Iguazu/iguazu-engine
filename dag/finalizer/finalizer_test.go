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

package finalizer

import (
	"math/big"
	"testing"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

// testDag is a blockchain stub with a DAG of headers and their finalized numbers.
type testDag struct {
	iface.BlockChain
	byHash map[common.Hash]*types.HeaderInfo
	byNr   map[uint64]*types.HeaderInfo
	lfNr   uint64
	nextID int64
}

func newTestDag() *testDag {
	return &testDag{byHash: map[common.Hash]*types.HeaderInfo{}, byNr: map[uint64]*types.HeaderInfo{}}
}

func (d *testDag) GetLastFinalizedNumber() uint64 { return d.lfNr }

func (d *testDag) GetHeaderByHash(h common.Hash) *types.HeaderInfo { return d.byHash[h] }

func (d *testDag) GetHeaderByNumber(nr uint64) *types.HeaderInfo { return d.byNr[nr] }

// add creates a block of the slot with the parents.
func (d *testDag) add(slot uint64, parents ...*types.HeaderInfo) *types.HeaderInfo {
	d.nextID++
	h := &types.HeaderInfo{
		Hash:         common.BigToHash(big.NewInt(d.nextID)),
		Slot:         slot,
		ParentHashes: common.HashArray{},
	}
	for _, p := range parents {
		h.ParentHashes = append(h.ParentHashes, p.Hash)
		if p.Height >= h.Height {
			h.Height = p.Height + 1
		}
	}
	d.byHash[h.Hash] = h
	return h
}

// finalize sets finalized numbers in the passed order.
func (d *testDag) finalize(headers ...*types.HeaderInfo) {
	for _, h := range headers {
		d.lfNr++
		h.Nr = d.lfNr
		d.byNr[d.lfNr] = h
	}
}

func assertForwardFinalization(t *testing.T, f *Finalizer, spines common.HashArray, base, wantBase common.Hash, wantRes common.HashArray) {
	t.Helper()
	res, bSpine, err := f.ForwardFinalization(spines, base)
	if err != nil {
		t.Fatalf("ForwardFinalization: %v", err)
	}
	if bSpine != wantBase {
		t.Fatalf("base spine: got %#x, want %#x", bSpine, wantBase)
	}
	if len(res) != len(wantRes) || (len(res) > 0 && !res.IsEqualTo(wantRes)) {
		t.Fatalf("spines: got %v, want %v", res, wantRes)
	}
}

// TestForwardFinalization covers the case when the chain of a spine contains
// ancestors from a parallel branch with slots greater than the previous spine slot.
func TestForwardFinalization(t *testing.T) {
	d := newTestDag()
	genesis := d.add(0)
	d.finalize(genesis)
	base := d.add(184, genesis)
	a1 := d.add(96, genesis)
	a2 := d.add(120, a1)
	a3 := d.add(152, a2)
	a4 := d.add(216, a3)
	s1 := d.add(280, a4)
	s2 := d.add(376, base)
	s3 := d.add(440, a1, base)
	s4 := d.add(568, s3)
	s5 := d.add(696, s2)
	d.finalize(base, a1, a2, a3, a4, s1, s2, s3, s4, s5)
	// not finalized spines
	s6 := d.add(728, s4, s5)
	notFinSpine := d.add(200, a3)
	unknown := common.HexToHash("0xff01")

	f := &Finalizer{bc: d}
	finSpines := common.HashArray{s1.Hash, s2.Hash, s3.Hash, s4.Hash, s5.Hash}

	t.Run("all spines finalized", func(t *testing.T) {
		assertForwardFinalization(t, f, finSpines, base.Hash, s5.Hash, common.HashArray{})
	})
	t.Run("last spine not finalized", func(t *testing.T) {
		assertForwardFinalization(t, f, append(finSpines.Copy(), s6.Hash), base.Hash, s5.Hash, common.HashArray{s6.Hash})
	})
	t.Run("spines partially finalized", func(t *testing.T) {
		assertForwardFinalization(t, f, common.HashArray{s1.Hash, s2.Hash, s6.Hash}, base.Hash, s2.Hash, common.HashArray{s6.Hash})
	})
	t.Run("unknown spine", func(t *testing.T) {
		assertForwardFinalization(t, f, common.HashArray{unknown, s1.Hash}, base.Hash, base.Hash, common.HashArray{unknown, s1.Hash})
	})
	t.Run("spine slot passed by finalized blocks", func(t *testing.T) {
		assertForwardFinalization(t, f, common.HashArray{notFinSpine.Hash, s1.Hash}, base.Hash, base.Hash, common.HashArray{notFinSpine.Hash, s1.Hash})
	})
}

// TestForwardFinalizationRollback checks that a mismatched finalization is detected
// and the base spine is not moved forward, so a rollback is performed.
func TestForwardFinalizationRollback(t *testing.T) {
	build := func() (d *testDag, base, p1, q1, s *types.HeaderInfo) {
		d = newTestDag()
		genesis := d.add(0)
		d.finalize(genesis)
		base = d.add(100, genesis)
		d.finalize(base)
		q1 = d.add(120, base)
		p1 = d.add(150, base)
		s = d.add(300, p1, q1)
		return d, base, p1, q1, s
	}

	t.Run("correct order", func(t *testing.T) {
		d, base, p1, q1, s := build()
		d.finalize(q1, p1, s)
		assertForwardFinalization(t, &Finalizer{bc: d}, common.HashArray{s.Hash}, base.Hash, s.Hash, common.HashArray{})
	})
	t.Run("extra local spine changes order", func(t *testing.T) {
		// p1 was finalized as a spine locally, so q1 got a greater number
		d, base, p1, q1, s := build()
		d.finalize(p1, q1, s)
		assertForwardFinalization(t, &Finalizer{bc: d}, common.HashArray{s.Hash}, base.Hash, base.Hash, common.HashArray{s.Hash})
	})
	t.Run("not ancestor finalized", func(t *testing.T) {
		d, base, p1, q1, s := build()
		z := d.add(130, base)
		d.finalize(q1, z, p1, s)
		assertForwardFinalization(t, &Finalizer{bc: d}, common.HashArray{s.Hash}, base.Hash, base.Hash, common.HashArray{s.Hash})
	})
	t.Run("same slot order", func(t *testing.T) {
		d := newTestDag()
		genesis := d.add(0)
		d.finalize(genesis)
		base := d.add(100, genesis)
		d.finalize(base)
		r1 := d.add(150, base)
		r2 := d.add(150, genesis)
		s := d.add(300, r1, r2)
		sorted := sortSlotHeaders([]*types.HeaderInfo{r1, r2})
		first, second := d.byHash[sorted[0]], d.byHash[sorted[1]]

		d.finalize(second, first, s)
		assertForwardFinalization(t, &Finalizer{bc: d}, common.HashArray{s.Hash}, base.Hash, base.Hash, common.HashArray{s.Hash})

		d.lfNr = base.Nr
		d.finalize(first, second, s)
		assertForwardFinalization(t, &Finalizer{bc: d}, common.HashArray{s.Hash}, base.Hash, s.Hash, common.HashArray{})
	})
}

func TestSortSlotHeaders(t *testing.T) {
	low := &types.HeaderInfo{Hash: common.HexToHash("0x01"), Height: 1}
	high := &types.HeaderInfo{Hash: common.HexToHash("0x02"), Height: 2}
	morePar := &types.HeaderInfo{Hash: common.HexToHash("0x03"), Height: 1, ParentHashes: common.HashArray{{1}, {2}}}
	sameA := &types.HeaderInfo{Hash: common.HexToHash("0x0a"), Height: 1}

	got := sortSlotHeaders([]*types.HeaderInfo{sameA, low, morePar, high})
	want := common.HashArray{high.Hash, morePar.Hash, low.Hash, sameA.Hash}
	if !got.IsEqualTo(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
