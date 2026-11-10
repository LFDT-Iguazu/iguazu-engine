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

// Package finalizer implements chain finalization of DAG network:
// - ordering (defines order of blocks in finalized chain)
// - apply transactions
// - state propagation
package finalizer

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync/atomic"
	"time"

	dagspine "github.com/LFDT-Iguazu/iguazu-engine/dag/spine"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

const (
	// CoordDelaySlots is the number of slots of delay to retrieve candidates for the coord network.
	CoordDelaySlots uint64 = 2
	// CreateDagSlotsLimit is the limit of slots in the DAG chain to stop block creation.
	CreateDagSlotsLimit = 8
)

// syncer is the minimal sync interface required by the finalizer.
type syncer interface {
	Synchronising() bool
}

// blockChain is the blockchain interface required by the finalizer.
// It is defined in iguazu-types so that gwat can implement it directly.
// finalizer uses only a subset of iface.BlockChain's methods, but a type
// satisfying iface.BlockChain automatically satisfies this alias.
type blockChain = iface.BlockChain

// Finalizer finalizes blocks in the DAG chain.
type Finalizer struct {
	bc     blockChain
	syncer syncer
	busy   int32 // non-zero while finalization is in progress
}

// New creates a new Finalizer instance.
func New(bc blockChain, s syncer) *Finalizer {
	f := &Finalizer{
		bc:     bc,
		syncer: s,
	}
	atomic.StoreInt32(&f.busy, 0)
	return f
}

// Finalize runs the finalization procedure for the given spines.
func (f *Finalizer) Finalize(spines *common.HashArray, baseSpine *common.Hash) error {
	if atomic.LoadInt32(&f.busy) == 1 {
		slog.Info("⌛ Finalization is skipped: process busy")
		return ErrBusy
	}

	atomic.StoreInt32(&f.busy, 1)
	defer atomic.StoreInt32(&f.busy, 0)

	if len(*spines) == 0 {
		slog.Info("⌛ Finalization is skipped: received spines empty")
		return nil
	}

	ctx := context.Background()
	lastFinBlock := f.bc.GetLastFinalizedBlock()

	if err := f.SetSpineState(baseSpine, lastFinBlock.Nr()); err != nil {
		return err
	}

	lastFinBlock = f.bc.GetLastFinalizedBlock()
	lastFinNr := lastFinBlock.Nr()
	successSpine := lastFinBlock.Hash()

	// collect and check finalizing blocks
	spinesMap := make(iface.SlotSpineMap, len(*spines))
	for _, spineHash := range *spines {
		block := f.bc.GetBlockByHash(spineHash)
		if block == nil {
			slog.Error("Finalization failed (spine not found)", "spineHash", spineHash.Hex(), "err", ErrSpineNotFound)
			return ErrSpineNotFound
		}
		spinesMap[block.Slot()] = block
	}

	// sort by slots
	slots := common.SorterAscU64{}
	for sl := range spinesMap {
		slots = append(slots, sl)
	}
	sort.Sort(slots)

	for _, slot := range slots {
		spine := spinesMap[slot]
		orderedChain, err := dagspine.GetDagChain(f.bc, spine)
		if err != nil {
			slog.Error("Finalization failed (calc fin seq)", "err", err)
			return err
		}

		slog.Info("Finalization spine chain calculated",
			"isSync", f.isSyncing(),
			"lfNr", lastFinNr,
			"slot", spine.Slot(),
			"height", spine.Height(),
			"hash", spine.Hash().Hex(),
			"chain", orderedChain.GetHashes(),
		)

		if len(orderedChain) == 0 {
			slog.Error("⌛ Finalization skip finalized spine: (must never happen)",
				"slot", spine.Slot(), "nr", spine.Nr(), "height", spine.Height(), "hash", spine.Hash().Hex(), "err", ErrEmptyFinSeq)
			return ErrEmptyFinSeq
		}

		// finalize blocks in order
		for i, block := range orderedChain {
			nr := lastFinNr + uint64(i) + 1
			isHead := i == len(orderedChain)-1
			var stateBlockHash common.Hash
			if i == 0 {
				stateBlockHash = lastFinBlock.Hash()
			} else {
				stateBlockHash = orderedChain[i-1].Hash()
			}
			if err := f.bc.FinalizeBlock(block.Hash(), nr, stateBlockHash, isHead); err != nil {
				slog.Error("Block finalization failed",
					"isHead", isHead, "calc.nr", nr, "slot", block.Slot(),
					"height", block.Height(), "hash", block.Hash().Hex(), "err", err)
				if err := f.SetSpineState(&successSpine, nr); err != nil {
					return err
				}
				return err
			}
			lastFinBlock = block
		}
		lastBlock := f.bc.GetBlock(ctx, orderedChain[len(orderedChain)-1].Hash())
		slog.Info("⛓ Finalization of spine completed",
			"blocks", len(orderedChain),
			"slot", lastBlock.Slot(),
			"calc.nr", lastFinNr,
			"nr", lastBlock.Nr(),
			"height", lastBlock.Height(),
			"hash", lastBlock.Hash().Hex(),
		)

		lastFinNr = lastBlock.Nr()
		f.updateTips(*orderedChain.GetHashes(), lastBlock)
		slog.Info("⛓ Finalization of spine completed (updateTips)",
			"blocks", len(orderedChain),
			"slot", lastBlock.Slot(),
			"calc.nr", lastFinNr,
			"nr", lastBlock.Nr(),
			"height", lastBlock.Height(),
			"hash", lastBlock.Hash().Hex(),
		)

		successSpine = spine.Hash()
	}
	return nil
}

// updateTips updates tips in accordance with finalized blocks.
func (f *Finalizer) updateTips(finHashes common.HashArray, lastBlock iface.Block) {
	f.bc.FinalizeTips(finHashes, lastBlock.Hash(), lastBlock.Height())
}

// isSyncing returns true if the sync process is running.
func (f *Finalizer) isSyncing() bool {
	return f.syncer.Synchronising()
}

// GetFinalizingCandidates returns the ordered DAG block hashes for finalization.
func (f *Finalizer) GetFinalizingCandidates(lteSlot *uint64) (common.HashArray, error) {
	tips := f.bc.GetTips()

	finChain := make(iface.Blocks, 0)
	for _, block := range f.bc.GetBlocksByHashes(tips.GetOrderedAncestorsHashes().Uniq()).ToArray() {
		if block.Height() > 0 && block.Nr() == 0 {
			finChain = append(finChain, block)
		}
	}

	if len(finChain) == 0 {
		return common.HashArray{}, nil
	}

	lastFinSlot := f.bc.GetLastFinalizedBlock().Slot()
	spines, err := dagspine.CalculateSpines(finChain, lastFinSlot)
	if err != nil {
		return common.HashArray{}, err
	}

	if lteSlot != nil {
		filtered := make(iface.SlotSpineMap)
		for slot, spine := range spines {
			if *lteSlot >= slot {
				filtered[slot] = spine
			}
		}
		return *filtered.GetOrderedHashes(), nil
	}
	return *spines.GetOrderedHashes(), nil
}

// IsValidSequenceOfSpines checks whether the given sequence of spines is valid.
func (f *Finalizer) IsValidSequenceOfSpines(spines common.HashArray) (bool, error) {
	if len(spines) == 0 {
		return true, nil
	}
	defer func(start time.Time) {
		slog.Info("TIME", "elapsed", time.Since(start).String(), "func", "ValidateCandidates", "spines", len(spines))
	}(time.Now())

	var (
		hasNotFin     = false
		dagCandidates common.HashArray
		optSpines     []common.HashArray
		err           error
	)

	mapHeaders := f.bc.GetHeadersByHashes(spines)
	for _, b := range mapHeaders {
		if b == nil {
			slog.Error("IsValidSequenceOfSpines header not found", "headers", mapHeaders)
			return false, nil
		}
		if b.Nr == 0 && b.Height > 0 {
			hasNotFin = true
		} else {
			if b.Nr != b.Height {
				slog.Warn("IsValidSequenceOfSpines b.Nr != b.Height", "b.Nr", b.Nr, "b.Height", b.Height, "hash", b.Hash)
			}
		}
	}

	if hasNotFin {
		fromSlot := f.bc.GetLastFinalizedHeader().Slot
		optSpines, err = f.bc.GetOptimisticSpines(fromSlot)
		if err != nil {
			slog.Error("GetOptimisticSpines error", "slot", fromSlot, "err", err)
			return false, err
		}
		if len(optSpines) == 0 {
			slog.Info("No spines found", "tips", f.bc.GetTips().Print(), "slot", fromSlot)
		}
		for _, candidate := range optSpines {
			if len(candidate) > 0 {
				dagCandidates = append(dagCandidates, candidate[0])
			}
		}
	}

	// check order by slots and gap in sequence
	var prevBlock *types.HeaderInfo
	for i, sp := range spines {
		bl := mapHeaders[sp]
		if i == 0 {
			prevBlock = bl
			continue
		}
		if bl.Slot <= prevBlock.Slot {
			return false, nil
		}
		if prevBlock.Nr == 0 && prevBlock.Height > 0 {
			checkPart := common.HashArray{prevBlock.Hash, bl.Hash}
			seqInter := dagCandidates.SequenceIntersection(checkPart)
			if !checkPart.IsEqualTo(seqInter) {
				return false, nil
			}
		} else {
			var followingSpine common.Hash
			for fnr := prevBlock.Nr; ; {
				fnr++
				nxt := f.bc.GetHeaderByNumber(fnr)
				if nxt == nil {
					spBlock := mapHeaders[sp]
					if spBlock.Nr == 0 && spBlock.Height > 0 && len(dagCandidates) > 0 {
						followingSpine = dagCandidates[0]
					}
					break
				}
				if nxt.Nr == nxt.Height {
					followingSpine = nxt.Hash
					break
				}
			}
			if sp != followingSpine {
				return false, nil
			}
		}
		prevBlock = bl
	}
	return true, nil
}

// SetSpineState sets state by past spine.
func (f *Finalizer) SetSpineState(spineHash *common.Hash, lfNr uint64) error {
	if spineHash == nil {
		slog.Error("Set spine state: bad param", "spineHash", nil)
		return ErrBadParams
	}
	return f.bc.RollbackFinalization(*spineHash, lfNr)
}

// ForwardFinalization recalculates finalization params by skipping correctly finalized spines.
func (f *Finalizer) ForwardFinalization(spines common.HashArray, baseSpine common.Hash) (common.HashArray, common.Hash, error) {
	spn, bSpine, err := f.forwardFinalization(&spines, &baseSpine)
	return *spn, *bSpine, err
}

func (f *Finalizer) forwardFinalization(spines *common.HashArray, baseSpine *common.Hash) (*common.HashArray, *common.Hash, error) {
	if baseSpine == nil || spines == nil || len(*spines) == 0 {
		return spines, baseSpine, nil
	}

	lfNr := f.bc.GetLastFinalizedNumber()
	baseHeader := f.bc.GetHeaderByHash(*baseSpine)
	if baseHeader == nil || baseHeader.Height > 0 && baseHeader.Nr == 0 {
		return spines, baseSpine, errInvalidBaseSpine
	}

	curNr := baseHeader.Nr
	curIndex := 0
	var (
		spineHeader *types.HeaderInfo
		spineHash   common.Hash
		// finalized blocks after the last confirmed spine
		segment = make([]*types.HeaderInfo, 0)
	)

	for nr := curNr + 1; nr <= lfNr && curIndex < len(*spines); nr++ {
		header := f.bc.GetHeaderByNumber(nr)
		if header == nil {
			return spines, baseSpine, fmt.Errorf("header not found by nr")
		}
		curSpine := (*spines)[curIndex]
		if spineHash != curSpine {
			spineHash = curSpine
			spineHeader = f.bc.GetHeaderByHash(curSpine)
		}
		// The finalization chain of a spine is ordered by slot and ends with the spine.
		// Its blocks may belong to parallel branches but never exceed the spine slot.
		if spineHeader == nil || header.Slot > spineHeader.Slot {
			break
		}
		if len(segment) > 0 && header.Slot < segment[len(segment)-1].Slot {
			break
		}
		segment = append(segment, header)
		if header.Hash != curSpine {
			continue
		}
		if !isSpineDagChain(segment) {
			break
		}
		curIndex++
		baseSpine = &curSpine
		segment = segment[:0]
	}

	resSpine := (*spines)[curIndex:]

	slog.Info("forward finalization: result",
		"baseSpine", fmt.Sprintf("%#x", baseSpine),
		"spines", resSpine,
	)

	return &resSpine, baseSpine, nil
}

// isSpineDagChain checks that a sequence of finalized blocks, ordered by slot and
// ending with the spine, is the finalization chain of that spine: it consists of the
// spine ancestors only and keeps the canonical order of blocks within a slot.
func isSpineDagChain(chain []*types.HeaderInfo) bool {
	if len(chain) == 0 {
		return false
	}
	// blocks of the same slot must keep the canonical order
	for i := 0; i < len(chain); {
		j := i + 1
		for j < len(chain) && chain[j].Slot == chain[i].Slot {
			j++
		}
		if j-i > 1 {
			for k, h := range sortSlotHeaders(chain[i:j]) {
				if chain[i+k].Hash != h {
					return false
				}
			}
		}
		i = j
	}
	// every block must be a spine ancestor reachable through the chain blocks only
	chainMap := make(map[common.Hash]*types.HeaderInfo, len(chain))
	for _, h := range chain {
		chainMap[h.Hash] = h
	}
	spine := chain[len(chain)-1]
	visited := map[common.Hash]struct{}{spine.Hash: {}}
	queue := []*types.HeaderInfo{spine}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, ph := range cur.ParentHashes {
			if _, ok := visited[ph]; ok {
				continue
			}
			if parent, ok := chainMap[ph]; ok {
				visited[ph] = struct{}{}
				queue = append(queue, parent)
			}
		}
	}
	return len(visited) == len(chain)
}

// sortSlotHeaders returns the hashes of blocks of one slot in the canonical order:
// height desc, number of parents desc, hash asc (as dagspine.SortBlocks orders blocks).
func sortSlotHeaders(headers []*types.HeaderInfo) common.HashArray {
	sorted := make([]*types.HeaderInfo, len(headers))
	copy(sorted, headers)
	sort.SliceStable(sorted, func(a, b int) bool {
		ha, hb := sorted[a], sorted[b]
		if ha.Height != hb.Height {
			return ha.Height > hb.Height
		}
		if len(ha.ParentHashes) != len(hb.ParentHashes) {
			return len(ha.ParentHashes) > len(hb.ParentHashes)
		}
		return ha.Hash.String() < hb.Hash.String()
	})
	hashes := make(common.HashArray, len(sorted))
	for i, h := range sorted {
		hashes[i] = h.Hash
	}
	return hashes
}

// errInvalidBaseSpine is returned when the base spine is not found or invalid.
var errInvalidBaseSpine = fmt.Errorf("invalid base spine")
