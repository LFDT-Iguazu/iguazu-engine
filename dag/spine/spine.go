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

// Package spine implements the DAG spine ordering algorithms.
package spine

import (
	"fmt"
	"sort"

	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

// BlockChain is the minimal blockchain interface required by the spine algorithms.
type BlockChain interface {
	GetBlocksByHashes(hashes common.HashArray) iface.BlockMap
	GetBlockFinalizedNumber(hash common.Hash) *uint64
}

// SortBlocks sorts blocks by (height desc, parentCount desc, hash asc).
// This deterministic ordering is used to select the canonical spine.
func SortBlocks(blocks []iface.Block) []iface.Block {
	heightPlenMap := map[uint64]map[uint64]map[common.Hash]iface.Block{}
	for _, bl := range blocks {
		h := bl.Height()
		plen := uint64(len(bl.ParentHashes()))
		hash := bl.Hash()
		if heightPlenMap[h] == nil {
			heightPlenMap[h] = map[uint64]map[common.Hash]iface.Block{}
		}
		if heightPlenMap[h][plen] == nil {
			heightPlenMap[h][plen] = map[common.Hash]iface.Block{}
		}
		heightPlenMap[h][plen][hash] = bl
	}

	heightKeys := make(common.SorterDescU64, 0, len(heightPlenMap))
	for k := range heightPlenMap {
		heightKeys = append(heightKeys, k)
	}
	sort.Sort(heightKeys)

	sortedBlocks := []iface.Block{}
	for _, hk := range heightKeys {
		plenMap := heightPlenMap[hk]
		plenKeys := make(common.SorterDescU64, 0, len(plenMap))
		for plk := range plenMap {
			plenKeys = append(plenKeys, plk)
		}
		sort.Sort(plenKeys)
		for _, k := range plenKeys {
			hashMap := plenMap[k]
			hashKeys := make(common.HashArray, 0, len(hashMap))
			for h := range hashMap {
				hashKeys = append(hashKeys, h)
			}
			hashKeys = hashKeys.Sort()
			for _, hash := range hashKeys {
				sortedBlocks = append(sortedBlocks, hashMap[hash])
			}
		}
	}
	return sortedBlocks
}

// CalculateSpines selects one canonical spine block per slot from the given blocks,
// excluding slots up to and including lastFinSlot.
func CalculateSpines(blocks iface.Blocks, lastFinSlot uint64) (iface.SlotSpineMap, error) {
	blocksBySlot, err := blocks.GroupBySlot()
	if err != nil {
		return nil, err
	}
	spines := make(iface.SlotSpineMap)
	slots := common.SorterAscU64{}
	for sl := range blocksBySlot {
		if sl > lastFinSlot {
			slots = append(slots, sl)
		}
	}
	sort.Sort(slots)
	for _, slot := range slots {
		slotBlocks := SortBlocks(blocksBySlot[slot])
		if len(slotBlocks) == 0 {
			continue
		}
		spines[slot] = slotBlocks[0]
	}
	return spines, nil
}

// GetDagChain collects all non-finalized ancestor blocks of spine in
// deterministic finalization order (ascending slot, canonical sort within slot).
func GetDagChain(bc BlockChain, spine iface.Block) (iface.Blocks, error) {
	candidatesInChain := make(map[common.Hash]struct{})
	dagBlocks := make(iface.Blocks, 0)
	if err := processBlock(bc, spine, candidatesInChain, &dagBlocks); err != nil {
		return nil, fmt.Errorf("calculate dag chain (process block): %w", err)
	}

	blocksBySlot, err := dagBlocks.GroupBySlot()
	if err != nil {
		return nil, fmt.Errorf("calculate dag chain (group by slot): %w", err)
	}

	slots := common.SorterAscU64{}
	for sl := range blocksBySlot {
		slots = append(slots, sl)
	}
	sort.Sort(slots)

	orderedBlocks := iface.Blocks{}
	for _, slot := range slots {
		slotBlocks := SortBlocks(blocksBySlot[slot])
		if len(slotBlocks) == 0 {
			continue
		}
		orderedBlocks = append(orderedBlocks, slotBlocks...)
	}

	if len(orderedBlocks) > 0 {
		if orderedBlocks[len(orderedBlocks)-1].Hash() != spine.Hash() {
			return nil, fmt.Errorf("calculate dag chain: bad sequence — spine not last")
		}
	}
	return orderedBlocks, nil
}

func calculateChain(bc BlockChain, block iface.Block, candidatesInChain map[common.Hash]struct{}) (iface.Blocks, error) {
	chain := make(iface.Blocks, 0, len(block.ParentHashes()))
	if err := processBlock(bc, block, candidatesInChain, &chain); err != nil {
		return nil, err
	}
	return chain, nil
}

func processBlock(bc BlockChain, block iface.Block, candidatesInChain map[common.Hash]struct{}, chain *iface.Blocks) error {
	if _, wasProcessed := candidatesInChain[block.Hash()]; wasProcessed || block.Number() != nil {
		return nil
	}
	parentsMap := bc.GetBlocksByHashes(block.ParentHashes())
	parents := make([]iface.Block, 0, len(parentsMap))
	for h, b := range parentsMap {
		if b == nil {
			return fmt.Errorf("block not found hash=%#x", h)
		}
		parents = append(parents, b)
	}
	sortedParents := SortBlocks(parents)

	candidatesInChain[block.Hash()] = struct{}{}
	for _, parent := range sortedParents {
		nr := bc.GetBlockFinalizedNumber(parent.Hash())
		if _, wasProcessed := candidatesInChain[parent.Hash()]; !wasProcessed && nr == nil {
			chainPart, err := calculateChain(bc, parent, candidatesInChain)
			if err != nil {
				return err
			}
			if len(chainPart) != 0 {
				*chain = append(*chain, chainPart...)
			}
			candidatesInChain[parent.Hash()] = struct{}{}
		}
	}
	*chain = append(*chain, block)
	return nil
}
