// Copyright (C) 2019-2020 Algorand, Inc.
// This file is part of go-algorand
//
// go-algorand is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// go-algorand is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with go-algorand.  If not, see <https://www.gnu.org/licenses/>.

package ledger

import (
	"fmt"

	"github.com/algorand/go-algorand/data/basics"
)

type globalAppStateChange uint64
type localAppStateChange uint64

const (
	noGlobalSC      globalAppStateChange = 0
	createdGlobalSC globalAppStateChange = 1
	deletedGlobalSC globalAppStateChange = 2
)

const (
	noLocalSC       localAppStateChange = 0
	optedInLocalSC  localAppStateChange = 1
	optedOutLocalSC localAppStateChange = 2
)

type globalAppDelta struct {
	stateChange globalAppStateChange
	creator     basics.Address
	params      basics.AppParams
	kvCow       keyValueCow
}

type localAppDelta struct {
	stateChange localAppStateChange
	kvCow       keyValueCow
}

func (cb *roundCowState) getLocalAppDelta(addr basics.Address, appIdx basics.AppIndex) (*localAppDelta, bool) {
	// Have we modified any local app state for this (account, app id)?
	delta, ok := cb.mods.appaccts[localAppKey{addr, appIdx}]
	return delta, ok
}

func (cb *roundCowState) ensureLocalAppDelta(addr basics.Address, appIdx basics.AppIndex) *localAppDelta {
	// Have we already modified any local app state for this account?
	delta, ok := cb.mods.appaccts[localAppKey{addr, appIdx}]
	if ok {
		return delta
	}

	// Initialize a localAppDelta to track any future changes
	delta = &localAppDelta{
		kvCow: makeKeyValueCow(),
	}
	cb.mods.appaccts[localAppKey{addr, appIdx}] = delta

	return delta
}

func (cb *roundCowState) getGlobalAppDelta(appIdx basics.AppIndex) (*globalAppDelta, bool) {
	delta, ok := cb.mods.appglob[appIdx]
	return delta, ok
}

func (cb *roundCowState) ensureGlobalAppDelta(appIdx basics.AppIndex) *globalAppDelta {
	delta, ok := cb.mods.appglob[appIdx]
	if ok {
		return delta
	}

	delta = &globalAppDelta{
		kvCow: makeKeyValueCow(),
	}
	cb.mods.appglob[appIdx] = delta

	return delta
}

func (cb *roundCowState) getAppParams(aidx basics.AppIndex) (basics.AppParams, basics.Address, bool, error) {
	delta, ok := cb.getGlobalAppDelta(aidx)
	if ok {
		if delta.stateChange == deletedGlobalSC {
			return basics.AppParams{}, basics.Address{}, false, nil
		}
		if delta.stateChange == createdGlobalSC {
			return delta.params, delta.creator, true, nil
		}
	}
	return cb.lookupParent.getAppParams(aidx)
}

func (cb *roundCowState) optedIn(addr basics.Address, appIdx basics.AppIndex) (bool, error) {
	// Check localAppDelta if present for this account, app id
	delta, ok := cb.getLocalAppDelta(addr, appIdx)
	if ok {
		if delta.stateChange == optedOutLocalSC {
			return false, nil
		}
		if delta.stateChange == optedInLocalSC {
			return true, nil
		}
	}
	return cb.lookupParent.optedIn(addr, appIdx)
}

func (cb *roundCowState) optIn(appIdx basics.AppIndex, addr basics.Address) error {
	// Make sure we're not already opted in
	optedIn, err := cb.optedIn(addr, appIdx)
	if err != nil {
		return err
	}
	if optedIn {
		return fmt.Errorf("cannot opt in: acct %v is already opted in to app %d", addr, appIdx)
	}

	// Ensure we have a local delta
	delta := cb.ensureLocalAppDelta(addr, appIdx)

	// Clear any existing kv delta, since we should always opt in from a
	// clean state
	delta.kvCow.clear()

	// Update state accordingly
	delta.stateChange = optedInLocalSC
	return nil
}

func (cb *roundCowState) optOut(appIdx basics.AppIndex, addr basics.Address) error {
	// Make sure we're opted in
	optedIn, err := cb.optedIn(addr, appIdx)
	if err != nil {
		return err
	}
	if !optedIn {
		return fmt.Errorf("cannot opt out: acct %v is not opted in to app %d", addr, appIdx)
	}

	// Ensure we have a local delta
	delta := cb.ensureLocalAppDelta(addr, appIdx)

	// Clear any existing kv delta, since opting out must completely clear
	// the key/value store
	delta.kvCow.clear()

	// Update state accordingly
	delta.stateChange = optedOutLocalSC
	return nil
}

func (cb *roundCowState) createApp(appIdx basics.AppIndex, creator basics.Address, params basics.AppParams) error {
	// Ensure app does not exist
	_, _, exists, err := cb.getAppParams(appIdx)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("cannot create app: app %d already exists", appIdx)
	}

	// Mark app as created
	delta := cb.ensureGlobalAppDelta(appIdx)
	delta.stateChange = createdGlobalSC
	delta.creator = creator
	delta.params = params
	return nil
}

func (cb *roundCowState) updateApp(appIdx basics.AppIndex, approvalProgram, clearStateProgram []byte) error {
	// Ensure app exists
	params, _, exists, err := cb.getAppParams(appIdx)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("cannot update app: app %d does not exist", appIdx)
	}

	// Update app
	delta := cb.ensureGlobalAppDelta(appIdx)

	// Copy program bytes, just in case the caller does something with the slice
	approv := make([]byte, len(approvalProgram))
	copy(approv, approvalProgram)

	clear := make([]byte, len(clearStateProgram))
	copy(clear, clearStateProgram)

	params.ApprovalProgram = approv
	params.ClearStateProgram = clear

	delta.params = params
	return nil
}

func (cb *roundCowState) deleteApp(appIdx basics.AppIndex) error {
	// Ensure app exists
	_, _, exists, err := cb.getAppParams(appIdx)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("cannot delete app: app %d does not exist", appIdx)
	}

	// Get/create a globalAppDelta for this app
	delta := cb.ensureGlobalAppDelta(appIdx)

	// Mark app as deleted
	delta.stateChange = deletedGlobalSC
	delta.creator = basics.Address{}
	delta.params = basics.AppParams{}
	return nil
}

func (cb *roundCowState) getLocal(addr basics.Address, appIdx basics.AppIndex, key string) (basics.TealValue, bool, error) {
	// Ensure we are opted in
	optedIn, err := cb.optedIn(addr, appIdx)
	if err != nil {
		return basics.TealValue{}, false, err
	}
	if !optedIn {
		err = fmt.Errorf("cannot read local key: acct %v is not opted in to app %d", addr, appIdx)
		return basics.TealValue{}, false, err
	}

	// Check if we have a local kv cow
	delta, ok := cb.getLocalAppDelta(addr, appIdx)
	if ok {
		// Check kv cow
		hitCow, tv, ok := delta.kvCow.read(key)
		if hitCow {
			return tv, ok, nil
		}

		// If this delta has opted us in, we should not check our
		// parent, since we were opted out before
		if delta.stateChange == optedInLocalSC {
			return basics.TealValue{}, false, nil
		}
	}

	// Fall back to parent
	return cb.lookupParent.getLocal(addr, appIdx, key)
}

func (cb *roundCowState) setLocal(addr basics.Address, appIdx basics.AppIndex, key string, value basics.TealValue) (err error) {
	// Ensure we are opted in
	optedIn, err := cb.optedIn(addr, appIdx)
	if err != nil {
		return err
	}
	if !optedIn {
		err = fmt.Errorf("cannot set local key: acct %v is not opted in to app %d", addr, appIdx)
		return err
	}

	// Ensure we have a kv cow
	delta := cb.ensureLocalAppDelta(addr, appIdx)

	// Write to the cow
	delta.kvCow.write(key, value)

	return nil
}

func (cb *roundCowState) delLocal(addr basics.Address, appIdx basics.AppIndex, key string) (err error) {
	// Ensure we are opted in
	optedIn, err := cb.optedIn(addr, appIdx)
	if err != nil {
		return err
	}
	if !optedIn {
		err = fmt.Errorf("cannot delete local key: acct %v is not opted in to app %d", addr, appIdx)
		return err
	}

	// Ensure we have a kv cow
	delta := cb.ensureLocalAppDelta(addr, appIdx)

	// Write to the cow
	delta.kvCow.del(key)

	return nil
}

func (cb *roundCowState) getGlobal(appIdx basics.AppIndex, key string) (basics.TealValue, bool, error) {
	// Ensure app exists
	_, _, exists, err := cb.getAppParams(appIdx)
	if err != nil {
		return basics.TealValue{}, false, err
	}
	if !exists {
		err = fmt.Errorf("cannot get global key: app %d does not exist", appIdx)
		return basics.TealValue{}, false, err
	}

	// Check if we have global modified app info/kv cow
	delta, ok := cb.getGlobalAppDelta(appIdx)
	if ok {
		// Check kv cow
		hitCow, tv, ok := delta.kvCow.read(key)
		if hitCow {
			return tv, ok, nil
		}

		// If this delta is creating the app, we should not check our
		// parent if the key is missing from the kv cow, since the
		// app did not exist before
		if delta.stateChange == createdGlobalSC {
			return basics.TealValue{}, false, nil
		}
	}

	// Fall back to parent
	return cb.lookupParent.getGlobal(appIdx, key)
}

func (cb *roundCowState) setGlobal(appIdx basics.AppIndex, key string, value basics.TealValue) (err error) {
	// Ensure app exists
	_, _, exists, err := cb.getAppParams(appIdx)
	if err != nil {
		return err
	}
	if !exists {
		err = fmt.Errorf("cannot set global key: app %d does not exist", appIdx)
		return err
	}

	// Ensure we have a kv cow
	delta := cb.ensureGlobalAppDelta(appIdx)

	// Write to the cow
	delta.kvCow.write(key, value)

	return nil
}

func (cb *roundCowState) delGlobal(appIdx basics.AppIndex, key string) (err error) {
	// Ensure app exists
	_, _, exists, err := cb.getAppParams(appIdx)
	if err != nil {
		return err
	}
	if !exists {
		err = fmt.Errorf("cannot delete global key: app %d does not exist", appIdx)
		return err
	}

	// Ensure we have a kv cow
	delta := cb.ensureGlobalAppDelta(appIdx)

	// Write to the cow
	delta.kvCow.del(key)

	return nil
}

func (cb *roundCowState) mergeGlobalAppDelta(aidx basics.AppIndex, cga *globalAppDelta) {
	// Grab delta reference for this app id (might be empty)
	pga := cb.ensureGlobalAppDelta(aidx)

	// Do some sanity checks
	if pga.stateChange == createdGlobalSC && cga.stateChange == createdGlobalSC {
		// App IDs are globally unique and monotonically increasing, so
		// creating an app with a given ID twice is impossible
		panic("invalid global app state change (created twice)!")
	}
	if pga.stateChange == deletedGlobalSC && cga.stateChange == deletedGlobalSC {
		// Deleting an app with the same ID twice is impossible
		panic("invalid global app state change (deleted twice)!")
	}
	if pga.stateChange == deletedGlobalSC && cga.stateChange == createdGlobalSC {
		// App IDs are globally unique and monotonically increasing, so
		// once an app is deleted, it should be impossible to create
		// one with the same ID again
		panic("invalid global app state change (deleted before created)!")
	}

	switch cga.stateChange {
	case deletedGlobalSC:
		// Sanity check: deleting child should have no delta
		if len(cga.kvCow.delta) != 0 {
			panic("delta deleted app, but kvCow had nonzero length delta")
		}

		// If the child deleted the app, replay that event
		err := cb.deleteApp(aidx)
		if err != nil {
			panic(fmt.Sprintf("unable to merge deleted app to parent: %v", err))
		}
	case createdGlobalSC:
		// Sanity check: parent should have no kv delta since app is being created
		if len(pga.kvCow.delta) != 0 {
			panic("delta created app, but parent cow parent had nonzero delta")
		}

		// If the child created the app, replay that event
		err := cb.createApp(aidx, cga.creator, cga.params)
		if err != nil {
			panic(fmt.Sprintf("unable to merge created app to parent: %v", err))
		}
	case noGlobalSC:
	default:
		panic(fmt.Sprintf("unknown globalAppStateChange %v", cga.stateChange))
	}

	// Reply key/value deltas in us
	for key, kvDelta := range cga.kvCow.delta {
		switch kvDelta.Action {
		case basics.SetUintAction:
			fallthrough
		case basics.SetBytesAction:
			val, ok := kvDelta.ToTealValue()
			if !ok {
				panic(fmt.Sprintf("failed to convert global kvDelta to value: %v", kvDelta))
			}

			err := cb.setGlobal(aidx, key, val)
			if err != nil {
				panic(fmt.Sprintf("failed to merge cow global k/v write: %v", err))
			}
		case basics.DeleteAction:
			err := cb.delGlobal(aidx, key)
			if err != nil {
				panic(fmt.Sprintf("failed to merge cow global k/v delete: %v", err))
			}
		default:
			panic(fmt.Sprintf("unknown global ValueDelta action %v", kvDelta.Action))
		}
	}
}

/*
The parent localAppDelta can be in one of three states:
	- noLocalSC (indicating no state change)
	- optInLocalSC (indicating we need to opt in)
	- optOutLocalSC (indicating we need to opt out)

The child localAppDelta can also be in one of those three states. There are
therefore nine possible transitions.

Both opting in and opting out represent a state change that completely clears
any existing values in the key/value store. So when a child is in one of those
states, we should

1. Set the parent's state change equal to the child's state change
2. Totally clear any key/value deltas from the parent
3. Copy any key/value deltas from the child into the parent

When the child is in the noLocalSC state, we should not totally overwrite the
parent's state, but instead just copy any key/value deltas from the child to
the parent (without clearing existing key/value deltas from the parent). For
example, the parent might be in optInLocalSC with some key/value deltas, and
the child might be in noLocalSC with some key/value deltas. We want to merge
these key/value deltas together.
*/

func (cb *roundCowState) mergeLocalAppDelta(addr basics.Address, aidx basics.AppIndex, cla *localAppDelta) {
	// Grab delta reference for this addr/aidx local state
	pla := cb.ensureLocalAppDelta(addr, aidx)

	switch cla.stateChange {
	case optedOutLocalSC:
		// Change our state to opted out, and clear any pending
		// key/value deltas we may have had.
		pla.stateChange = optedOutLocalSC
		pla.kvCow.clear()

		// Sanity check: opting out, child should have no kv delta
		if len(cla.kvCow.delta) != 0 {
			panic("child opted out, but child kvCow had nonzero length delta")
		}
	case optedInLocalSC:
		// Change our state to opted in, and clear any pending
		// key/value deltas we may have had
		pla.stateChange = optedInLocalSC
		pla.kvCow.clear()
	case noLocalSC:
		// Parent *must* be opted in if len(cla.kvCow.delta) > 0.
	default:
		panic(fmt.Sprintf("unknown localAppStateChange %v", cla.stateChange))
	}

	// Replay any deltas from child state in parent
	for key, kvDelta := range cla.kvCow.delta {
		switch kvDelta.Action {
		case basics.SetUintAction:
			fallthrough
		case basics.SetBytesAction:
			val, ok := kvDelta.ToTealValue()
			if !ok {
				panic(fmt.Sprintf("failed to convert local kvDelta to value: %v", kvDelta))
			}

			err := cb.commitParent.setLocal(addr, aidx, key, val)
			if err != nil {
				panic(fmt.Sprintf("failed to merge cow local k/v write: %v", err))
			}
		case basics.DeleteAction:
			err := cb.commitParent.delLocal(addr, aidx, key)
			if err != nil {
				panic(fmt.Sprintf("failed to merge cow local k/v delete: %v", err))
			}
		default:
			panic(fmt.Sprintf("unknown local ValueDelta action %v", kvDelta.Action))
		}
	}
}
