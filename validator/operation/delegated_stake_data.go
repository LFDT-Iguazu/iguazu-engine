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
// of delegating stake functionalities.

/*
Package operation implements all operations related to Iguazu validators (Deposit, Withdrawal, Exit).
*/
package operation

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/LFDT-Iguazu/iguazu-types/common"
	"github.com/LFDT-Iguazu/iguazu-types/crypto"
	"github.com/LFDT-Iguazu/iguazu-types/rlp"
)

var (
	errDelegatingStakeNilValBin = errors.New("delegate stake binary data conforms to nil instance")
)

var (
	minDsdLen = minDelegatingStakeDataLen() // Minimum binary length for DelegatingStakeData
)

// DelegatingStakeData represents the data structure for managing delegation of stakes to validators.
type DelegatingStakeData struct {
	Rules       DelegatingStakeRules `json:"rules"`       // Rules applied after the trial period.
	TrialPeriod uint64               `json:"trialPeriod"` // Duration of the trial period in slots.
	TrialRules  DelegatingStakeRules `json:"trialRules"`  // Rules applied during the trial period.
}

// init initializes the DelegatingStakeData structure with the provided rules and trial information.
// Validates both the main rules and trial rules, ensuring correct configuration for delegation.
func (dsd *DelegatingStakeData) init(
	rules *DelegatingStakeRules,
	trialPeriod uint64,
	trialRules *DelegatingStakeRules,
) error {
	if rules == nil {
		rules = &DelegatingStakeRules{}
	} else if err := rules.Validate(); err != nil {
		return fmt.Errorf("delegate rules err: %w", err)
	}

	if trialRules == nil {
		trialRules = &DelegatingStakeRules{}
	}
	// Validate trial rules if a trial period is specified.
	if trialPeriod > 0 && len(trialRules.ProfitShare()) > 0 {
		if err := trialRules.ValidateProfitShare(); err != nil {
			return fmt.Errorf("delegate trial rules err: %w", err)
		}
	}
	if trialPeriod > 0 && len(trialRules.StakeShare()) > 0 {
		if err := trialRules.ValidateStakeShare(); err != nil {
			return fmt.Errorf("delegate trial rules err: %w", err)
		}
	}

	dsd.Rules = *rules
	dsd.TrialPeriod = trialPeriod
	dsd.TrialRules = *trialRules
	return nil
}

// NewDelegatingStakeData creates a new instance of DelegatingStakeData with the provided rules and trial period.
func NewDelegatingStakeData(
	rules *DelegatingStakeRules,
	trialPeriod uint64,
	trialRules *DelegatingStakeRules,
) (*DelegatingStakeData, error) {
	dsd := DelegatingStakeData{}
	if err := dsd.init(rules, trialPeriod, trialRules); err != nil {
		return nil, err
	}
	return &dsd, nil
}

// NewDelegatingStakeDataFromBinary creates a new DelegatingStakeData instance from binary data.
// Returns nil if the binary data corresponds to a nil instance.
func NewDelegatingStakeDataFromBinary(bin []byte) (*DelegatingStakeData, error) {
	dsd := &DelegatingStakeData{}
	err := dsd.UnmarshalBinary(bin)
	// Check if the binary data corresponds to a nil instance.
	if errors.Is(err, errDelegatingStakeNilValBin) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return dsd, nil
}

// rlpDelegatingStakeOperation is an internal helper structure used for encoding and decoding stake data.
type rlpDelegatingStakeOperation struct {
	R  []byte // Binary-encoded rules
	TP uint64 // Trial period duration
	TR []byte // Binary-encoded trial rules
}

// MarshalBinary encodes the DelegatingStakeData structure into a binary format.
func (dsd *DelegatingStakeData) MarshalBinary() ([]byte, error) {
	if dsd == nil {
		return make([]byte, common.Uint32Size), nil
	}

	bRules, err := dsd.Rules.MarshalBinary()
	if err != nil {
		return nil, err
	}
	bTRules, err := dsd.TrialRules.MarshalBinary()
	if err != nil {
		return nil, err
	}
	rd := &rlpDelegatingStakeOperation{
		R:  bRules,
		TP: dsd.TrialPeriod,
		TR: bTRules,
	}
	enc, err := rlp.EncodeToBytes(rd)
	if err != nil {
		return nil, err
	}
	binData := make([]byte, common.Uint32Size+len(enc))
	// Set the length of the encoded data.
	binary.BigEndian.PutUint32(binData[:common.Uint32Size], uint32(len(enc)))
	// Set the encoded data.
	copy(binData[common.Uint32Size:], enc)
	return binData, nil
}

// UnmarshalBinary decodes binary data into a DelegatingStakeData structure.
func (dsd *DelegatingStakeData) UnmarshalBinary(b []byte) error {
	if len(b) < minDsdLen {
		// Check if the binary data corresponds to a nil instance.
		if bytes.Equal(b, make([]byte, common.Uint32Size)) {
			return errDelegatingStakeNilValBin
		}
		return ErrBadDataLen
	}
	dataLen := int(binary.BigEndian.Uint32(b[0:common.Uint32Size]))
	if len(b) < common.Uint32Size+dataLen {
		return ErrBadDataLen
	}

	rop := &rlpDelegatingStakeOperation{}
	if err := rlp.DecodeBytes(b[common.Uint32Size:common.Uint32Size+dataLen], rop); err != nil {
		return err
	}

	dsd.TrialPeriod = rop.TP

	dsd.Rules = DelegatingStakeRules{}
	if err := dsd.Rules.UnmarshalBinary(rop.R); err != nil {
		return err
	}
	dsd.TrialRules = DelegatingStakeRules{}
	if err := dsd.TrialRules.UnmarshalBinary(rop.TR); err != nil {
		return err
	}
	return nil
}

// Copy creates a deep copy of the DelegatingStakeData instance.
func (dsd *DelegatingStakeData) Copy() *DelegatingStakeData {
	if dsd == nil {
		return nil
	}
	rules := dsd.Rules.Copy()
	tRules := dsd.TrialRules.Copy()
	return &DelegatingStakeData{
		Rules:       *rules,
		TrialPeriod: dsd.TrialPeriod,
		TrialRules:  *tRules,
	}
}

// IsEmpty checks whether the DelegatingStakeData has no rules defined.
func (dsd *DelegatingStakeData) IsEmpty() bool {
	if dsd == nil {
		return true
	}
	return len(dsd.Rules.Withdrawal()) == 0
}

// minDelegatingStakeDataLen calculates the minimum length of binary data for a DelegatingStakeData instance.
func minDelegatingStakeDataLen() int {
	emptyBin, err := (&DelegatingStakeData{}).MarshalBinary()
	if err != nil {
		panic("Validator: calc min delegate stake binary data length failed")
	}
	return len(emptyBin)
}

func normalizeDelegateData(data *DelegatingStakeData) map[string]interface{} {
	return map[string]interface{}{
		"rules":        normalizeRules(&data.Rules),
		"trial_period": data.TrialPeriod,
		"trial_rules":  normalizeRules(&data.TrialRules),
	}
}

func normalizeRules(rules *DelegatingStakeRules) map[string]interface{} {
	exit := make([]string, len(rules.Exit()))
	for i, addr := range rules.Exit() {
		exit[i] = strings.ToLower(addr.Hex())
	}

	withdrawal := make([]string, len(rules.Withdrawal()))
	for i, addr := range rules.Withdrawal() {
		withdrawal[i] = strings.ToLower(addr.Hex())
	}

	return map[string]interface{}{
		"exit":         exit,
		"profit_share": normalizeAddressMap(rules.ProfitShare()),
		"stake_share":  normalizeAddressMap(rules.StakeShare()),
		"withdrawal":   withdrawal,
	}
}

func normalizeAddressMap(data map[common.Address]uint8) map[string]uint8 {
	result := make(map[string]uint8, len(data))
	for addr, v := range data {
		result[strings.ToLower(addr.Hex())] = v
	}
	return result
}

// toCanonicalJSON returns compact JSON with recursively sorted object keys,
// matching the JS toCanonicalJson function used for hash computation.
func toCanonicalJSON(v interface{}) string {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			keyJSON, _ := json.Marshal(k)
			parts = append(parts, string(keyJSON)+":"+toCanonicalJSON(val[k]))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case map[string]uint8:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			keyJSON, _ := json.Marshal(k)
			parts = append(parts, string(keyJSON)+":"+fmt.Sprintf("%d", val[k]))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []string:
		parts := make([]string, len(val))
		for i, s := range val {
			b, _ := json.Marshal(s)
			parts[i] = string(b)
		}
		sort.Strings(parts)
		return "[" + strings.Join(parts, ",") + "]"
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}

func computeDelegateHash(delegateData *DelegatingStakeData) ([]byte, error) {
	canonical := strings.ToLower(toCanonicalJSON(normalizeDelegateData(delegateData)))
	return crypto.Keccak256([]byte(canonical)), nil
}
