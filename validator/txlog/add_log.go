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

/*
Package txlog is used for creating logs related to validator operations,
such as deposits, withdrawals, exits, and updates, and adding them to the blockchain state.
Additionally, it provides marshalling and unmarshalling functions for each log type,
enabling efficient encoding and decoding of log data.
*/
package txlog

import (
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	"github.com/LFDT-Iguazu/iguazu-types/common"
	"github.com/LFDT-Iguazu/iguazu-types/crypto"
)

var (
	//Events signatures.
	EvtDepositLogSignature    = common.Hash(crypto.Keccak256Hash([]byte("DepositLog")))
	EvtExitReqLogSignature    = common.Hash(crypto.Keccak256Hash([]byte("ExitRequestLog")))
	EvtWithdrawalLogSignature = common.Hash(crypto.Keccak256Hash([]byte("WithdrawaRequestLog")))
	//validator sync op
	EvtActivateLogSignature      = common.Hash(crypto.Keccak256Hash([]byte("activate")))
	EvtDeactivateLogSignature    = common.Hash(crypto.Keccak256Hash([]byte("deactivate")))
	EvtUpdateBalanceLogSignature = common.Hash(crypto.Keccak256Hash([]byte("update-balance")))
	EvtDelegatingStakeSignature  = common.Hash(crypto.Keccak256Hash([]byte("delegating-stake")))
)

type LogEntry struct {
	indexed bool
	data    []byte
}

type EventEmmiter struct {
	state iface.StateDB
}

func NewEventEmmiter(state iface.StateDB) *EventEmmiter {
	return &EventEmmiter{state: state}
}

func (e *EventEmmiter) Deposit(evtAddr common.Address, data []byte, logsEntries ...*LogEntry) {
	e.addLog(evtAddr, EvtDepositLogSignature, data, logsEntries...)
}

func (e *EventEmmiter) ExitRequest(evtAddr common.Address, data []byte, logsEntries ...*LogEntry) {
	e.addLog(evtAddr, EvtExitReqLogSignature, data, logsEntries...)
}

func (e *EventEmmiter) WithdrawalRequest(evtAddr common.Address, data []byte, logsEntries ...*LogEntry) {
	e.addLog(evtAddr, EvtWithdrawalLogSignature, data, logsEntries...)
}

func (e *EventEmmiter) NewIndexedAddressLogEntry(address common.Address) *LogEntry {
	return &LogEntry{
		indexed: true,
		data:    address.Hash().Bytes(),
	}
}

func (e *EventEmmiter) addLog(targetAddr common.Address, signature common.Hash, data []byte, logsEntries ...*LogEntry) {
	topics := []common.Hash{signature}

	for _, entry := range logsEntries {
		if entry.indexed {
			topics = append(topics, common.BytesToHash(entry.data))
		} else {
			data = append(data, entry.data...)
		}
	}

	e.state.AddLog(&iface.LogEntry{
		Address: [20]byte(targetAddr),
		Topics:  toRawTopics(topics),
		Data:    data,
	})
}

func toRawTopics(topics []common.Hash) [][32]byte {
	raw := make([][32]byte, len(topics))
	for i, t := range topics {
		raw[i] = [32]byte(t)
	}
	return raw
}
