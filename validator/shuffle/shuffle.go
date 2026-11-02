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

// Package shuffle computes the pseudorandom permutation of validators used to
// assign block creators to slots.
//
// The permutation is the "swap-or-not" shuffle of the Ethereum consensus
// specification (compute_shuffled_index), applied to a whole list at once:
// every round picks a pivot and, for each pair of positions (i, (pivot-i) mod
// n), swaps them when the bit selected by the larger position is set. Hashes
// are SHA-256.
package shuffle

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"time"

	"github.com/LFDT-Iguazu/iguazu-types/common"
	"github.com/minio/sha256-simd"
)

const (
	// rounds is the number of swap-or-not rounds (SHUFFLE_ROUND_COUNT).
	rounds = 90
	// maxListSize bounds the list length so positions fit the hash input.
	maxListSize = 1 << 40
)

// ShuffleValidators permutes validators in place, using seed as the source of
// randomness, and returns the same slice. The rounds are applied in reverse
// order (from the last round to the first).
func ShuffleValidators(validators []common.Address, seed common.Hash) ([]common.Address, error) {
	start := time.Now()

	if uint64(len(validators)) > maxListSize {
		return nil, fmt.Errorf("list size %d out of bounds", len(validators))
	}
	for round := rounds - 1; round >= 0; round-- {
		applyRound(validators, seed, byte(round))
	}

	slog.Info("^^^^^^^^^^^^ TIME", "elapsed", time.Since(start), "func", "ShuffleValidators")
	return validators, nil
}

// applyRound performs one swap-or-not round on list.
func applyRound(list []common.Address, seed common.Hash, round byte) {
	n := uint64(len(list))
	if n <= 1 {
		return
	}

	// The hash input is seed || round || uint32_le(position / 256).
	var input [common.HashLength + 1 + 4]byte
	copy(input[:], seed[:])
	input[common.HashLength] = round

	pivotDigest := sha256.Sum256(input[:common.HashLength+1])
	pivot := binary.LittleEndian.Uint64(pivotDigest[:8]) % n

	// Each source hash covers 256 consecutive positions; positions are visited
	// out of order, so the hashes are cached per block.
	sources := make(map[uint64][32]byte)
	bit := func(position uint64) bool {
		block := position >> 8
		source, ok := sources[block]
		if !ok {
			binary.LittleEndian.PutUint32(input[common.HashLength+1:], uint32(block))
			source = sha256.Sum256(input[:])
			sources[block] = source
		}
		return source[(position&0xff)>>3]>>(position&7)&1 == 1
	}

	for i := uint64(0); i < n; i++ {
		flip := (pivot + n - i) % n
		// Visit each pair once; the bit is chosen by the larger position.
		if i < flip && bit(flip) {
			list[i], list[flip] = list[flip], list[i]
		}
	}
}

// Bytes8 returns x as 8 little-endian bytes.
func Bytes8(x uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, x)
	return b
}

// FromBytes8 decodes 8 little-endian bytes. Shorter input yields 0.
func FromBytes8(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}
