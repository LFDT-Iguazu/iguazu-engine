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

package shuffle

import (
	"crypto/sha256"
	"encoding/binary"
	"io"
	"log/slog"
	"testing"

	"github.com/LFDT-Iguazu/iguazu-types/common"
)

// shuffledIndex is a direct transcription of compute_shuffled_index from the
// Ethereum consensus specification, used as an independent reference.
func shuffledIndex(index, n uint64, seed common.Hash) uint64 {
	for round := 0; round < rounds; round++ {
		in := append(seed.Bytes(), byte(round))
		h := sha256.Sum256(in)
		pivot := binary.LittleEndian.Uint64(h[:8]) % n
		flip := (pivot + n - index) % n
		position := index
		if flip > position {
			position = flip
		}
		var pos [4]byte
		binary.LittleEndian.PutUint32(pos[:], uint32(position/256))
		source := sha256.Sum256(append(append(seed.Bytes(), byte(round)), pos[:]...))
		if source[(position%256)/8]>>(position%8)&1 == 1 {
			index = flip
		}
	}
	return index
}

func addresses(n int) []common.Address {
	list := make([]common.Address, n)
	for i := range list {
		list[i] = common.BytesToAddress(Bytes8(uint64(i)))
	}
	return list
}

func TestShuffleMatchesSpec(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, n := range []int{2, 3, 10, 100, 255, 256, 257, 1000} {
		seed := common.BytesToHash(Bytes8(uint64(n) * 7919))
		in := addresses(n)
		out, err := ShuffleValidators(addresses(n), seed)
		if err != nil {
			t.Fatal(err)
		}
		for i := range out {
			if out[i] != in[shuffledIndex(uint64(i), uint64(n), seed)] {
				t.Fatalf("n=%d: position %d does not match compute_shuffled_index", n, i)
			}
		}
	}
}

func TestShuffleInPlace(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, n := range []int{0, 1} {
		out, err := ShuffleValidators(addresses(n), common.Hash{})
		if err != nil || len(out) != n {
			t.Fatalf("n=%d: got %d elements, %v", n, len(out), err)
		}
	}
	list := addresses(50)
	out, err := ShuffleValidators(list, common.HexToHash("0x01"))
	if err != nil {
		t.Fatal(err)
	}
	if &out[0] != &list[0] {
		t.Fatal("the list must be shuffled in place")
	}
}

func TestBytes8(t *testing.T) {
	for _, x := range []uint64{0, 1, 0x0102030405060708, ^uint64(0)} {
		if got := FromBytes8(Bytes8(x)); got != x {
			t.Errorf("FromBytes8(Bytes8(%d)) = %d", x, got)
		}
	}
	if b := Bytes8(1); b[0] != 1 || b[7] != 0 {
		t.Errorf("Bytes8 is not little-endian: %x", b)
	}
	if FromBytes8([]byte{1, 2, 3}) != 0 {
		t.Error("FromBytes8 of short input must be 0")
	}
}
