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

// Package operation implements operations related to validators.
// This package includes modules for specific validator operations, including the implementation
// of delegating stake rules functionalities.

package operation

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/LFDT-Iguazu/iguazu-types/common"
	"github.com/LFDT-Iguazu/iguazu-types/rlp"
	"github.com/prysmaticlabs/go-bitfield"
)

// DelegatingStakeRules represents the rules for delegating stakes to validators.
// It manages the participant addresses, their profit shares, stake shares, and rights to initiate exit or withdrawal procedures.
type DelegatingStakeRules struct {
	addrs       []common.Address
	profitShare []uint8
	stakeShare  []uint8
	exit        bitfield.Bitlist
	withdrawal  bitfield.Bitlist
}

// NewDelegatingStakeRules creates a new instance of DelegatingStakeRules based on provided data.
func NewDelegatingStakeRules(
	profitShare map[common.Address]uint8,
	stakeShare map[common.Address]uint8,
	exit []common.Address,
	withdrawal []common.Address,
) *DelegatingStakeRules {
	dsr := DelegatingStakeRules{}
	dsr.init(profitShare, stakeShare, exit, withdrawal)

	return &dsr
}

func (dr *DelegatingStakeRules) init(
	profitShare map[common.Address]uint8,
	stakeShare map[common.Address]uint8,
	exit []common.Address,
	withdrawal []common.Address,
) {
	type adrInfo struct {
		Profit   uint8
		Stake    uint8
		Exit     bool
		Withdraw bool
	}
	var (
		addrMap = make(map[common.Address]*adrInfo)
		addrs   []common.Address
	)
	// collect data
	for a, v := range profitShare {
		if v == 0 {
			continue
		}
		if _, ok := addrMap[a]; !ok {
			addrMap[a] = new(adrInfo)
		}
		addrMap[a].Profit = v
	}
	for a, v := range stakeShare {
		if v == 0 {
			continue
		}
		if _, ok := addrMap[a]; !ok {
			addrMap[a] = new(adrInfo)
		}
		addrMap[a].Stake = v
	}
	for _, a := range exit {
		if _, ok := addrMap[a]; !ok {
			addrMap[a] = new(adrInfo)
		}
		addrMap[a].Exit = true
	}
	for _, a := range withdrawal {
		if _, ok := addrMap[a]; !ok {
			addrMap[a] = new(adrInfo)
		}
		addrMap[a].Withdraw = true
	}
	// set data
	addrs = make([]common.Address, len(addrMap))
	addrCount := 0
	for adr := range addrMap {
		addrs[addrCount] = adr
		addrCount++
	}
	sort.Slice(addrs, func(i, j int) bool { return bytes.Compare(addrs[i][:], addrs[j][:]) < 0 })
	dr.addrs = addrs

	dr.profitShare = make([]uint8, len(addrs))
	dr.stakeShare = make([]uint8, len(addrs))
	dr.exit = bitfield.NewBitlist(uint64(len(addrs)))
	dr.withdrawal = bitfield.NewBitlist(uint64(len(addrs)))
	for i, adr := range addrs {
		info := addrMap[adr]
		if info.Profit > 0 {
			dr.profitShare[i] = info.Profit
		}
		if info.Stake > 0 {
			dr.stakeShare[i] = info.Stake
		}
		if info.Exit {
			dr.exit.SetBitAt(uint64(i), true)
		}
		if info.Withdraw {
			dr.withdrawal.SetBitAt(uint64(i), true)
		}
	}
}

// Validate checks that all rules are correctly configured.
func (dr *DelegatingStakeRules) Validate() error {
	if err := dr.ValidateProfitShare(); err != nil {
		return err
	}
	if err := dr.ValidateStakeShare(); err != nil {
		return err
	}
	if err := dr.validateExit(); err != nil {
		return err
	}
	if err := dr.validateWithdrawal(); err != nil {
		return err
	}
	return nil
}

// ValidateProfitShare ensures that the total profit share equals 100%.
func (dr *DelegatingStakeRules) ValidateProfitShare() error {
	var totalPrc uint
	for _, v := range dr.profitShare {
		totalPrc += uint(v)
	}
	if totalPrc != 100 {
		return ErrBadProfitShare
	}
	return nil
}

// ValidateStakeShare ensures that the total stake share equals 100%.
func (dr *DelegatingStakeRules) ValidateStakeShare() error {
	var totalPrc uint
	for _, v := range dr.stakeShare {
		totalPrc += uint(v)
	}
	if totalPrc != 100 {
		return ErrBadStakeShare
	}
	return nil
}

func (dr *DelegatingStakeRules) validateExit() error {
	if len(dr.Exit()) == 0 {
		return ErrNoExitRoles
	}
	return nil
}

func (dr *DelegatingStakeRules) validateWithdrawal() error {
	if len(dr.Withdrawal()) == 0 {
		return ErrNoWithdrawalRoles
	}
	return nil
}

// Copy creates a deep copy of the DelegatingStakeRules instance.
func (dr *DelegatingStakeRules) Copy() *DelegatingStakeRules {
	if dr == nil {
		return nil
	}
	var (
		adrCount    = len(dr.addrs)
		addrs       = make([]common.Address, adrCount)
		profitShare = make([]uint8, adrCount)
		stakeShare  = make([]uint8, adrCount)
		exit        = bitfield.NewBitlist(uint64(adrCount))
		withdrawal  = bitfield.NewBitlist(uint64(adrCount))
	)

	for i, adr := range dr.addrs {
		copy(addrs[i][:], adr[:])
		profitShare[i] = dr.profitShare[i]
		stakeShare[i] = dr.stakeShare[i]
	}

	copy(exit[:], dr.exit[:])
	if len(dr.exit) == 0 {
		exit = make(bitfield.Bitlist, 0)
	}
	copy(withdrawal[:], dr.withdrawal[:])
	if len(dr.withdrawal) == 0 {
		withdrawal = make(bitfield.Bitlist, 0)
	}

	return &DelegatingStakeRules{
		addrs:       addrs,
		profitShare: profitShare,
		stakeShare:  stakeShare,
		exit:        exit,
		withdrawal:  withdrawal,
	}
}

// ProfitShare returns a map of participant profit shares as percentages.
func (dr *DelegatingStakeRules) ProfitShare() map[common.Address]uint8 {
	data := map[common.Address]uint8{}
	for i, a := range dr.addrs {
		if v := dr.profitShare[i]; v > 0 {
			data[a] = v
		}
	}
	return data
}

// StakeShare returns a map of participant stake shares as percentages.
func (dr *DelegatingStakeRules) StakeShare() map[common.Address]uint8 {
	data := map[common.Address]uint8{}
	for i, a := range dr.addrs {
		if v := dr.stakeShare[i]; v > 0 {
			data[a] = v
		}
	}
	return data
}

// Exit returns a list of addresses authorized to initiate the exit procedure.
func (dr *DelegatingStakeRules) Exit() []common.Address {
	ixs := dr.exit.BitIndices()
	data := make([]common.Address, len(ixs))
	for i, ix := range ixs {
		data[i] = dr.addrs[ix]
	}
	return data
}

// Withdrawal returns a list of addresses authorized to initiate the withdrawal procedure.
func (dr *DelegatingStakeRules) Withdrawal() []common.Address {
	ixs := dr.withdrawal.BitIndices()
	data := make([]common.Address, len(ixs))
	for i, ix := range ixs {
		data[i] = dr.addrs[ix]
	}
	return data
}

type rlpDelegatingStakeRules struct {
	A []common.Address
	P []uint8
	S []uint8
	E []byte
	W []byte
}

// MarshalBinary encodes the DelegatingStakeRules into binary format.
func (dr *DelegatingStakeRules) MarshalBinary() ([]byte, error) {
	rd := &rlpDelegatingStakeRules{
		A: dr.addrs,
		P: dr.profitShare,
		S: dr.stakeShare,
		E: dr.exit,
		W: dr.withdrawal,
	}
	return rlp.EncodeToBytes(rd)
}

// UnmarshalBinary decodes binary data into a DelegatingStakeRules instance.
func (dr *DelegatingStakeRules) UnmarshalBinary(b []byte) error {
	rd := &rlpDelegatingStakeRules{}
	if err := rlp.DecodeBytes(b, rd); err != nil {
		return err
	}
	dr.addrs = rd.A
	dr.profitShare = rd.P
	dr.stakeShare = rd.S
	dr.exit = rd.E
	dr.withdrawal = rd.W
	return nil
}

// MarshalJSON encodes the DelegatingStakeRules into JSON format.
func (dr *DelegatingStakeRules) MarshalJSON() ([]byte, error) {
	data := struct {
		ProfitShare map[common.Address]uint8 `json:"profitShare"`
		StakeShare  map[common.Address]uint8 `json:"stakeShare"`
		Exit        []common.Address         `json:"exit"`
		Withdrawal  []common.Address         `json:"withdrawal"`
	}{
		ProfitShare: dr.ProfitShare(),
		StakeShare:  dr.StakeShare(),
		Exit:        dr.Exit(),
		Withdrawal:  dr.Withdrawal(),
	}
	return json.Marshal(data)
}
