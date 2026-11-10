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

// Package dag implements the Iguazu consensus:
// - consensus functionality
// - finalizing process
// - block creation process
package dag

import (
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/LFDT-Iguazu/iguazu-engine/dag/finalizer"
	"github.com/LFDT-Iguazu/iguazu-engine/dag/slotticker"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/iface"
	"github.com/LFDT-Iguazu/iguazu-types/blockdag/types"
	"github.com/LFDT-Iguazu/iguazu-types/common"
)

var (
	errSynchronization  = errors.New("synchronization")
	ErrInvalidBaseSpine = errors.New("invalid base spine")
	// errCheckpointSpineNotFound is returned when a coordinated checkpoint refers
	// to a spine block that is not in the local chain yet.
	errCheckpointSpineNotFound = errors.New("checkpoint spine not found")
)

// blockChain is the blockchain interface required by dag.
// It is defined in iguazu-types so that gwat can implement it directly.
type blockChain = iface.BlockChain

// Backend wraps all methods required by the Dag.
type Backend interface {
	BlockChain() blockChain
	Downloader() iface.Downloader
	Creator() iface.BlockCreator
	IsDevMode() bool
}

// Dag orchestrates DAG consensus: slot ticking, block creation, and finalization.
type Dag struct {
	eth        Backend
	bc         blockChain
	downloader iface.Downloader
	creator    iface.BlockCreator
	finalizer  *finalizer.Finalizer

	lastFinApiSlot uint64

	exitChan chan struct{}
	enddChan chan struct{}

	checkpoint *types.Checkpoint
}

// New creates a new Dag instance.
func New(eth Backend) *Dag {
	bc := eth.BlockChain()
	dl := eth.Downloader()
	d := &Dag{
		eth:        eth,
		bc:         bc,
		downloader: dl,
		creator:    eth.Creator(),
		finalizer:  finalizer.New(bc, dl),
		exitChan:   make(chan struct{}),
		enddChan:   make(chan struct{}),
	}
	return d
}

// Creator returns the block creator.
func (d *Dag) Creator() iface.BlockCreator {
	return d.creator
}

// HandleFinalize runs the finalization procedure for the given params.
func (d *Dag) HandleFinalize(data *types.FinalizationParams) *types.FinalizationResult {
	res := &types.FinalizationResult{
		Error: nil,
	}
	start := time.Now()

	if d.bc.GetSlotInfo() == nil {
		errStr := "no slot info"
		res.Error = &errStr
		slog.Error("Handle Finalize: response (no slot info)", "result", res, "err", errStr)
		return res
	}
	d.setLastFinalizeApiSlot()

	if d.isCpUnacceptable(data.Checkpoint) {
		lfHeader := d.bc.GetLastFinalizedHeader()
		lfHash := lfHeader.Hash
		res.LFSpine = &lfHash
		if cp := d.bc.GetLastCoordinatedCheckpoint(); cp != nil {
			res.CpEpoch = &cp.FinEpoch
			res.CpRoot = &cp.Root
		}
		slog.Warn("Handle Finalize: ignore bad finalization",
			"result", res,
			"resEpoch", *res.CpEpoch,
			"resSpine", res.LFSpine.Hex(),
			"resRoot", res.CpRoot.Hex(),
		)
		return res
	}

	// skip if synchronising
	if d.downloader.Synchronising() {
		errStr := errSynchronization.Error()
		res.Error = &errStr
		slog.Error("Handle Finalize: response (busy)", "result", res, "err", errStr)
	}

	d.bc.DagMuLock()
	defer d.bc.DagMuUnlock()

	if data.BaseSpine != nil {
		slog.Info("Handle Finalize: start",
			"data.SyncMode", data.SyncMode,
			"cp.FinEpoch", data.Checkpoint.FinEpoch,
			"cp.Epoch", data.Checkpoint.Epoch,
			"cp.BaseSpine", fmt.Sprintf("%#x", data.Checkpoint.Spine),
			"cp.Root", fmt.Sprintf("%#x", data.Checkpoint.Root),
			"baseSpine", fmt.Sprintf("%#x", data.BaseSpine),
			"spines", data.Spines,
			"ValSyncData", len(data.ValSyncData))
	} else {
		slog.Info("Handle Finalize: start",
			"data.SyncMode", data.SyncMode,
			"cp.FinEpoch", data.Checkpoint.FinEpoch,
			"cp.Epoch", data.Checkpoint.Epoch,
			"cp.BaseSpine", fmt.Sprintf("%#x", data.Checkpoint.Spine),
			"baseSpine", nil,
			"spines", data.Spines)
	}

	var err error
	baseSpine := *data.BaseSpine
	spines := data.Spines
	// if baseSpine is in spines - remove
	if bi := spines.IndexOf(baseSpine); bi >= 0 {
		spines = spines[bi+1:]
	}
	// forward finalization
	spines, baseSpine, err = d.finalizer.ForwardFinalization(spines, baseSpine)
	if err != nil {
		e := err.Error()
		res.Error = &e
		slog.Error("Handle Finalize: forward finalization failed", "syncMode", data.SyncMode, "err", err)
		return res
	}

	// fix bad validators state
	valSyncData := d.bc.FixValidatorSyncOps(data.Checkpoint.FinEpoch, data.ValSyncData)

	switch data.SyncMode {
	case types.NoSync:
		if err = d.handleSyncUnloadedBlocks(baseSpine, spines, data.Checkpoint); err != nil {
			strErr := err.Error()
			res.Error = &strErr
			slog.Error("Handle Finalize: response (sync failed)", "syncMode", data.SyncMode, "result", res, "err", err)
			return res
		}
	case types.MainSync:
		for i, vs := range valSyncData {
			slog.Info("Handle Finalize: valSync: main sync append", "i", i, "valSyncData", vs.Print())
		}
		d.bc.AppendNotProcessedValidatorSyncData(valSyncData)
		if !d.needMainSync(spines) {
			slog.Info("Handle Finalize: main sync skipped (all blocks loaded)", "baseSpine", baseSpine.Hex(), "spines", len(spines))
			break
		}
		if err = d.downloader.MainSync(baseSpine, spines); err != nil {
			strErr := err.Error()
			res.Error = &strErr
			slog.Error("Handle Finalize: response (sync failed)", "syncMode", data.SyncMode, "result", res, "err", err)
			return res
		}
	case types.HeadSync:
		for i, vs := range valSyncData {
			slog.Info("Handle Finalize: valSync: head sync append", "i", i, "valSyncData", vs.Print())
		}
		d.bc.AppendNotProcessedValidatorSyncData(valSyncData)
		if err = d.downloader.DagSync(data.Checkpoint.Spine, spines); err != nil {
			strErr := err.Error()
			res.Error = &strErr
			slog.Error("Handle Finalize: response (sync failed)", "syncMode", data.SyncMode, "result", res, "err", err)
			return res
		}
	default:
		e := fmt.Sprintf("bad sync mode=%d", data.SyncMode)
		res.Error = &e
		slog.Error("Handle Finalize: bad sync mode", "syncMode", data.SyncMode, "result", res)
		return res
	}

	// finalization
	if len(spines) > 0 {
		if err = d.finalizer.Finalize(&spines, &baseSpine); err != nil {
			if err == finalizer.ErrInsertUncompletedDag || err == finalizer.ErrSpineNotFound {
				slog.Error("Handle Finalize: response (finalize failed)", "result", res, "err", err)
				return res
			}
			e := err.Error()
			res.Error = &e
		} else {
			if err := d.applyCoordinatedCheckpoint(data.Checkpoint); err != nil {
				strErr := err.Error()
				res.Error = &strErr
				slog.Error("Handle Finalize: update era failed 1", "syncMode", data.SyncMode, "result", res, "err", err)
				return res
			}
		}
	} else {
		if err := d.applyCoordinatedCheckpoint(data.Checkpoint); err != nil {
			strErr := err.Error()
			res.Error = &strErr
			slog.Error("Handle Finalize: update era failed 2", "syncMode", data.SyncMode, "result", res, "err", err)
			return res
		}
	}

	// handle validator sync data
	if data.SyncMode == types.NoSync {
		for i, vs := range valSyncData {
			slog.Info("Handle Finalize: valSync", "i", i, "valSyncData", vs.Print())
		}
		d.bc.AppendNotProcessedValidatorSyncData(valSyncData)
	}

	lfHeader := d.bc.GetLastFinalizedHeader()
	lfHash := lfHeader.Hash
	res.LFSpine = &lfHash

	if cp := d.bc.GetLastCoordinatedCheckpoint(); cp != nil {
		res.CpEpoch = &cp.FinEpoch
		res.CpRoot = &cp.Root
	}
	if data.SyncMode == types.NoSync || data.SyncMode == types.HeadSync {
		d.bc.SetIsSynced(true)
		slog.Info("HandleFinalize SetIsSynced true", "SyncMode", data.SyncMode)
	}

	slog.Info("Handle Finalize: response",
		"resEpoch", *res.CpEpoch,
		"resSpine", res.LFSpine.Hex(),
		"resRoot", res.CpRoot.Hex(),
	)
	slog.Info("TIME", "elapsed", time.Since(start).String(), "func", "Finalize")
	return res
}

// applyCoordinatedCheckpoint stores the coordinated checkpoint and starts its era.
// A checkpoint whose spine block is not in the local chain is rejected before it is
// stored: such a checkpoint cannot be applied, and once persisted it breaks the
// node on the next start. The caller retries after the spine is synced.
func (d *Dag) applyCoordinatedCheckpoint(cp *types.Checkpoint) error {
	if cp == nil || d.bc.GetHeaderByHash(cp.Spine) == nil {
		return errCheckpointSpineNotFound
	}
	d.bc.SetLastCoordinatedCheckpoint(cp)
	return d.bc.HandleEra(cp)
}

// HandleSyncSpines runs blocks finalization procedure for spine sync.
func (d *Dag) HandleSyncSpines(spines common.HashArray) (bool, error) {
	start := time.Now()
	defer func(start time.Time) {
		slog.Info("Handle SpineSync: TIME", "elapsed", time.Since(start).String(), "func", "HandleSyncSpines")
	}(start)

	if len(spines) <= 1 {
		return false, nil
	}
	if d.bc.GetSlotInfo() == nil {
		err := fmt.Errorf("no slot info")
		slog.Error("Handle SpineSync: response (no slot info)", "err", err)
		return false, err
	}
	if !d.bc.IsSynced() {
		err := fmt.Errorf("not synced")
		slog.Error("Handle SpineSync: response (not synced)", "err", err)
		return false, err
	}
	// skip if synchronising
	if d.downloader.Synchronising() {
		err := errSynchronization
		slog.Error("Handle SpineSync: response (synchronization)", "err", err)
		return false, err
	}

	baseSpine := spines[0]
	// if baseSpine is in spines - remove
	if bi := spines.IndexOf(baseSpine); bi >= 0 {
		spines = spines[bi+1:]
	}
	slog.Info("Handle SpineSync: start",
		"baseSpine", fmt.Sprintf("%#x", baseSpine),
		"spines", spines,
	)
	// based on NoSync
	baseHeader := d.bc.GetHeaderByHash(baseSpine)
	if baseHeader == nil || baseHeader.Nr == 0 && baseHeader.Height > 0 {
		slog.Error("Handle SpineSync: response (check unloaded)", "err", ErrInvalidBaseSpine)
		return false, ErrInvalidBaseSpine
	}
	isSync, err := d.hasUnloadedBlocks(spines)
	if err != nil {
		slog.Error("Handle SpineSync: response (check unloaded)", "err", err)
		return false, err
	}
	if !isSync {
		slog.Info("Handle SpineSync: response (no unloaded)")
		return true, nil
	}

	if err = d.downloader.OptimisticSpineSync(spines); err != nil {
		slog.Error("Handle SpineSync: response (sync failed)", "err", err)
		return false, err
	}
	slog.Info("Handle SpineSync: end", "spines", spines)
	return true, nil
}

// handleSyncUnloadedBlocks checks whether synchronization is required and starts it.
func (d *Dag) handleSyncUnloadedBlocks(baseSpine common.Hash, spines common.HashArray, cp *types.Checkpoint) error {
	defer func(start time.Time) {
		slog.Info("TIME", "elapsed", time.Since(start).String(), "func", "dag.handleSyncUnloadedBlocks")
	}(time.Now())

	if len(spines) == 0 {
		return nil
	}
	baseHeader := d.bc.GetHeaderByHash(baseSpine)
	if baseHeader == nil || baseHeader.Nr == 0 && baseHeader.Height > 0 {
		return ErrInvalidBaseSpine
	}
	isSync, err := d.hasUnloadedBlocks(spines)
	if err != nil {
		return err
	}
	if !isSync {
		return nil
	}
	d.bc.SetIsSynced(false)

	d.bc.SetSyncCheckpointCache(cp)
	defer d.bc.ResetSyncCheckpointCache()
	if err = d.downloader.OptimisticSpineSync(spines); err != nil {
		return err
	}
	return nil
}

// needMainSync reports whether a main sync must fetch the spine blocks from peers.
// Peers may not serve blocks the node already has, and each such request waits for
// a timeout, so the sync is skipped when every block of the spines is loaded. The
// check is an optimization only: if it fails, the main sync runs, otherwise the same
// finalization request would fail again and again.
func (d *Dag) needMainSync(spines common.HashArray) bool {
	hasUnloaded, err := d.hasUnloadedBlocks(spines)
	if err != nil {
		slog.Warn("Handle Finalize: check unloaded blocks failed, run main sync", "err", err)
		return true
	}
	return hasUnloaded
}

func (d *Dag) hasUnloadedBlocks(spines common.HashArray) (bool, error) {
	defer func(start time.Time) {
		slog.Info("TIME", "elapsed", time.Since(start).String(), "func", "dag.hasUnloadedBlocks")
	}(time.Now())

	var (
		unl common.HashArray
		err error
	)
	for _, spine := range spines.Reverse() {
		spHeader := d.bc.GetHeaderByHash(spine)
		if spHeader == nil {
			return true, nil
		}
		if spHeader.Height == 0 {
			continue
		}
		_, _, unl, err = d.bc.CollectAncestorsAftCpByParents(spHeader.ParentHashes, spHeader.CpHash)
		if err != nil {
			return false, err
		}
		if len(unl) > 0 {
			return true, nil
		}
	}

	return false, nil
}

// HandleCoordinatedState returns the current coordinated state.
func (d *Dag) HandleCoordinatedState() *types.FinalizationResult {
	// skip if synchronising
	if d.downloader.Synchronising() {
		errStr := errSynchronization.Error()
		return &types.FinalizationResult{
			Error: &errStr,
		}
	}

	lfHeader := d.bc.GetLastFinalizedHeader()
	lfHash := lfHeader.Hash
	res := &types.FinalizationResult{
		Error:   nil,
		LFSpine: &lfHash,
	}
	var cpepoch uint64
	if cp := d.bc.GetLastCoordinatedCheckpoint(); cp != nil {
		res.CpEpoch = &cp.FinEpoch
		res.CpRoot = &cp.Root
		cpepoch = cp.FinEpoch
	}
	slog.Info("Handle CoordinatedState: response", "epoch", cpepoch, "result", res)
	return res
}

// HandleGetCandidates collects the next finalization candidates.
func (d *Dag) HandleGetCandidates(slot uint64) *types.CandidatesResult {
	// skip if synchronising
	if d.downloader.Synchronising() {
		errStr := errSynchronization.Error()
		return &types.CandidatesResult{
			Error: &errStr,
		}
	}

	defer func(tstart time.Time) {
		slog.Info("TIME", "elapsed", time.Since(tstart).String(), "func", "GetCandidates")
	}(time.Now())

	fromSlot := d.bc.GetLastFinalizedHeader().Slot
	optSpines, err := d.bc.GetOptimisticSpines(fromSlot)
	if len(optSpines) == 0 {
		slog.Info("No spines found", "tips", d.bc.GetTips().Print(), "slot", slot)
	}

	var candidates common.HashArray
	for _, candidate := range optSpines {
		if len(candidate) > 0 {
			header := d.bc.GetHeaderByHash(candidate[0])
			if header.Slot <= slot {
				candidates = append(candidates, candidate[0])
			}
		}
	}

	if len(candidates) == 0 {
		slog.Info("No candidates found", "slot", slot)
	}

	slog.Debug("Candidates HandleGetCandidates: get finalizing candidates",
		"err", err, "toSlot", slot, "fromSlot", fromSlot, "candidates", candidates)
	res := &types.CandidatesResult{
		Error:      nil,
		Candidates: candidates,
	}
	if err != nil {
		estr := err.Error()
		res.Error = &estr
	}
	return res
}

// HandleGetOptimisticSpines returns optimistic spines reachable from fromSpine.
func (d *Dag) HandleGetOptimisticSpines(fromSpine common.Hash) *types.OptimisticSpinesResult {
	// skip if synchronising
	if d.downloader.Synchronising() {
		errStr := errSynchronization.Error()
		return &types.OptimisticSpinesResult{
			Error: &errStr,
		}
	}

	tstart := time.Now()

	slog.Info("Handle GetOptimisticSpines: start", "fromSpine", fromSpine.Hex())

	spineBlock := d.bc.GetHeaderByHash(fromSpine)
	if spineBlock == nil {
		err := errors.New("bad params: base spine not found").Error()
		return &types.OptimisticSpinesResult{
			Error: &err,
			Data:  nil,
		}
	}

	spines, err := d.bc.GetOptimisticSpines(spineBlock.Slot)
	if len(spines) == 0 {
		slog.Info("No spines for tips", "tips", d.bc.GetTips().Print())
	}

	res := &types.OptimisticSpinesResult{
		Error: nil,
		Data:  spines,
	}
	if err != nil {
		estr := err.Error()
		res.Error = &estr
	}
	slog.Info("Handle GetOptimisticSpines: response",
		"result", len(res.Data), "elapsed", time.Since(tstart).String())
	return res
}

// HandleSyncSlotInfo sets the initial slot info to start head sync with the coordinating network.
func (d *Dag) HandleSyncSlotInfo(slotInfo types.SlotInfo) (bool, error) {
	slog.Info("Handle Sync Slot info", "params", slotInfo)
	si := d.bc.GetSlotInfo()
	if si == nil {
		err := d.bc.SetSlotInfo(d.slotInfoFromCoordinator(&slotInfo))
		if err != nil {
			return false, err
		}
		return true, nil
	} else if si.MatchCoordinatorParams(&slotInfo, si.CurrentSlot()) {
		return true, nil
	} else if d.eth.IsDevMode() {
		err := d.bc.SetSlotInfo(d.slotInfoFromCoordinator(&slotInfo))
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// slotInfoFromCoordinator completes the slot settings received from the coordinating
// network with the local slot duration fork settings: the coordinator is not aware of
// the fork and never reports them.
func (d *Dag) slotInfoFromCoordinator(slotInfo *types.SlotInfo) *types.SlotInfo {
	si := slotInfo.Copy()
	si.ForkSlotUpSecondsPerSlot = d.bc.Config().ForkSlotUpSecondsPerSlot
	si.UpSecondsPerSlot = d.bc.Config().UpSecondsPerSlot
	return si
}

// HandleValidateFinalization validates the given spine sequence by checking
// existence and ascending slot order.
func (d *Dag) HandleValidateFinalization(spines common.HashArray) (bool, error) {
	slog.Info("Handle validate finalisation: start", "spines", spines)

	headers := d.bc.GetBlocksByHashes(spines)
	lastSlot := uint64(0)

	for _, s := range spines {
		spine := headers[s]
		if spine == nil {
			err := fmt.Errorf("spine not found: %#x", s)
			slog.Warn("Handle validate finalisation: failed", "err", err)
			return false, err
		}
		if lastSlot >= spine.Slot() {
			err := fmt.Errorf("bad order by slot: %#x slot=%d prevSlot=%d", s, spine.Slot(), lastSlot)
			slog.Warn("Handle validate finalisation: failed", "err", err)
			return false, err
		}
	}
	return true, nil
}

// HandleValidateSpines validates the given candidate spine sequence.
func (d *Dag) HandleValidateSpines(spines common.HashArray) (bool, error) {
	slog.Debug("Candidates HandleValidateSpines req", "candidates", spines)
	return d.finalizer.IsValidSequenceOfSpines(spines)
}

// StartWork starts the Dag work loop, waiting for genesis before entering the slot loop.
func (d *Dag) StartWork() {
	startTicker := time.NewTicker(2000 * time.Millisecond)

	tickSec := 0
	for {
		select {
		case <-d.exitChan:
			d.exitProcedure()
			return
		case <-startTicker.C:
			si := d.bc.GetSlotInfo()
			currentTime := time.Now()
			if si != nil {
				genesisTime := time.Unix(int64(d.bc.GetSlotInfo().GenesisTime), 0)

				if currentTime.Before(genesisTime) {
					if tickSec != currentTime.Second() && currentTime.Second()%5 == 0 {
						timeRemaining := genesisTime.Sub(currentTime)
						slog.Info("Time before start", "hour", timeRemaining.Truncate(time.Second))
					}
				} else {
					slog.Info("Chain genesis time reached", "curSlot", si.CurrentSlot())
					startTicker.Stop()
					go d.workLoop()
					return
				}
				tickSec = currentTime.Second()
			}
		}
	}
}

// StopWork signals the Dag to stop and waits for it to exit cleanly.
func (d *Dag) StopWork() {
	d.exitChan <- struct{}{}
	<-d.enddChan
	close(d.enddChan)
}

func (d *Dag) exitProcedure() {
	d.creator.Stop()
	d.downloader.Terminate()
	d.bc.DagMuLock()
	d.bc.DagMuUnlock()
	close(d.exitChan)
	d.enddChan <- struct{}{}
}

func (d *Dag) workLoop() {
	slotTicker := slotticker.NewSlotTicker(d.bc.GetSlotInfo())
	// clean invalid not processed val sync pool
	d.bc.CleanInvalidNotProcessedValidatorSync()

	for {
		select {
		case <-d.exitChan:
			slotTicker.Done()
			d.exitProcedure()
			return
		case slot := <-slotTicker.C():
			if slot == 0 {
				d.bc.SetIsSynced(true)
				d.resetCheckpoint()
				continue
			}
			if !d.bc.IsSynced() {
				slog.Info("dag workloop !d.bc.IsSynced()", "slot", slot, "IsSynced", d.bc.IsSynced())
				d.resetCheckpoint()
				continue
			}
			if d.isCoordinatorConnectionLost() {
				d.bc.SetIsSynced(false)
				slog.Info("Detected coordinator skipped slot handling: sync mode on",
					"slot", slot, "coordSlot", d.getLastFinalizeApiSlot())
				continue
			}
			currentEpoch := d.bc.GetSlotInfo().SlotToEpoch(d.bc.GetSlotInfo().CurrentSlot())
			slog.Debug("curEpoch to eraInfo toEpoch",
				"epoch", currentEpoch, "eraToEpoch", d.bc.GetEraInfo().ToEpoch())

			var (
				err          error
				slotCreators []common.Address
			)

			startTransitionSlot, err := d.bc.GetSlotInfo().SlotOfEpochStart(
				d.bc.GetEraInfo().ToEpoch() + 1 - d.bc.Config().TransitionPeriod)
			if err != nil {
				slog.Error("Error calculating start transition slot", "error", err)
				return
			}

			endTransitionSlot, err := d.bc.GetSlotInfo().SlotOfEpochEnd(d.bc.GetEraInfo().ToEpoch())
			if err != nil {
				slog.Error("Error calculating end transition slot", "error", err)
			}

			slog.Info("New slot",
				"slot", slot,
				"currentEpoch", d.bc.GetSlotInfo().SlotToEpoch(slot),
				"era", d.bc.GetEraInfo().Number(),
				"startTransEpoch", d.bc.GetEraInfo().ToEpoch()+1-d.bc.Config().TransitionPeriod,
				"startTransSlot", startTransitionSlot,
				"endTransEpoch", d.bc.GetEraInfo().ToEpoch(),
				"endTransSlot", endTransitionSlot,
			)

			slotCreators, err = d.bc.ValidatorStorage().GetCreatorsBySlot(slot)
			if err != nil {
				slog.Error("Create block: get creators failed", "slot", slot, "error", err)
				continue
			}

			go d.work(slot, slotCreators)
		}
	}
}

func (d *Dag) work(slot uint64, slotCreators []common.Address) {
	if !d.bc.IsSynced() {
		return
	}
	if d.isSlotLocked(slot) {
		return
	}

	d.bc.DagMuLock()
	defer d.bc.DagMuUnlock()

	if d.Creator().IsRunning() {
		var canCreate bool
		for _, account := range slotCreators {
			if d.Creator().IsCreatorActive(account) {
				canCreate = true
				break
			}
		}
		if canCreate {
			tips := d.bc.GetTips()
			checkpoint := d.getCheckpoint()
			if checkpoint == nil {
				checkpoint = d.bc.GetLastCoordinatedCheckpoint()
			}

			err := d.Creator().RunBlockCreation(slot, slotCreators, tips, checkpoint)
			if err != nil {
				slog.Error("Create block error", "error", err)
			}
		}
	} else {
		slog.Warn("Creator stopped")
	}

	d.saveCheckpoint(d.bc.GetLastCoordinatedCheckpoint())

	if d.bc.GetSlotInfo().CurrentSlot() == slot && slot%d.bc.Config().SlotsPerEpoch == d.bc.Config().SlotsPerEpoch/2 {
		d.bc.CleanInvalidNotProcessedValidatorSync()
	}
}

// getLastFinalizeApiSlot returns the slot of the last HandleFinalize API call.
func (d *Dag) getLastFinalizeApiSlot() uint64 {
	return atomic.LoadUint64(&d.lastFinApiSlot)
}

// setLastFinalizeApiSlot records the current slot as the last finalize API call slot.
func (d *Dag) setLastFinalizeApiSlot() {
	slot := uint64(0)
	if si := d.bc.GetSlotInfo(); si != nil {
		slot = si.CurrentSlot()
	}
	atomic.StoreUint64(&d.lastFinApiSlot, slot)
}

// isCoordinatorConnectionLost returns true if the difference between the current slot
// and the last coord slot is greater than one epoch.
func (d *Dag) isCoordinatorConnectionLost() bool {
	slot := uint64(0)
	si := d.bc.GetSlotInfo()
	if si == nil {
		return true
	}
	slot = si.CurrentSlot()
	return (slot - d.getLastFinalizeApiSlot()) > si.SlotsPerEpoch
}

func (d *Dag) getCheckpoint() *types.Checkpoint {
	return d.checkpoint
}

func (d *Dag) saveCheckpoint(cp *types.Checkpoint) {
	d.checkpoint = cp
}

func (d *Dag) resetCheckpoint() {
	d.checkpoint = nil
}

// isSlotLocked returns true if the incoming slot is already finalized.
func (d *Dag) isSlotLocked(slot uint64) bool {
	return slot <= d.bc.GetLastFinalizedHeader().Slot
}

// isCpUnacceptable returns true if the provided checkpoint is on the hard-coded blocklist.
func (d *Dag) isCpUnacceptable(cp *types.Checkpoint) bool {
	if cp == nil {
		return true
	}
	conf := d.bc.Config()
	if len(conf.AcceptCpRootOnFinEpoch) == 0 {
		return false
	}
	if fEpochs, ok := conf.AcceptCpRootOnFinEpoch[cp.Root]; ok {
		for _, ep := range fEpochs {
			if cp.FinEpoch == ep {
				return false
			}
		}
		return true
	}
	return false
}
