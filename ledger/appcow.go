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

type modifiedGlobalApp struct {
	stateChange globalAppStateChange
	creator     basics.Address
	params      basics.AppParams
	kvCow       keyValueCow
}

type modifiedLocalApp struct {
	stateChange localAppStateChange
	kvCow       keyValueCow
}

func (cb *roundCowState) getModLocalApp(addr basics.Address, appIdx basics.AppIndex) (*modifiedLocalApp, bool) {
	// Have we modified any local app state for this account?
	modLocalApps, ok := cb.mods.appaccts[addr]
	if ok {
		// Within this account, have we modified any local app state for this app?
		modLocalApp, ok := modLocalApps[appIdx]
		if ok {
			return modLocalApp, true
		}
	}
	return nil, false
}

func (cb *roundCowState) deleteModLocalApp(addr basics.Address, appIdx basics.AppIndex) {
	// Have we modified any local app state for this account?
	modLocalApps, ok := cb.mods.appaccts[addr]
	if !ok {
		return
	}

	// Within this account, have we modified any local app state for this app?
	_, ok = modLocalApps[appIdx]
	if ok {
		// If so, delete it
		delete(modLocalApps, appIdx)
	}

	// Are there any app deltas left for this account? If not, clear it out
	if len(modLocalApps) == 0 {
		delete(cb.mods.appaccts, addr)
	}
}

func (cb *roundCowState) ensureModLocalApp(addr basics.Address, appIdx basics.AppIndex) *modifiedLocalApp {
	// Have we already modified any local app state for this account?
	modLocalApps, ok := cb.mods.appaccts[addr]
	if !ok {
		modLocalApps = make(map[basics.AppIndex]*modifiedLocalApp)
		cb.mods.appaccts[addr] = modLocalApps
	}

	// If we have an existing *modifiedLocalApp, return it
	modLocalApp, ok := modLocalApps[appIdx]
	if ok {
		return modLocalApp
	}

	// Initialize a modifiedLocalApp to track any future changes
	modLocalApp = &modifiedLocalApp{
		kvCow: makeKeyValueCow(),
	}
	modLocalApps[appIdx] = modLocalApp

	return modLocalApp
}

func (cb *roundCowState) getModGlobalApp(appIdx basics.AppIndex) (*modifiedGlobalApp, bool) {
	modGlobalApp, ok := cb.mods.appglob[appIdx]
	return modGlobalApp, ok
}

func (cb *roundCowState) ensureModGlobalApp(appIdx basics.AppIndex) *modifiedGlobalApp {
	modGlobalApp, ok := cb.mods.appglob[appIdx]
	if ok {
		return modGlobalApp
	}

	modGlobalApp = &modifiedGlobalApp{
		kvCow: makeKeyValueCow(),
	}
	cb.mods.appglob[appIdx] = modGlobalApp

	return modGlobalApp
}

func (cb *roundCowState) deleteModGlobalApp(appIdx basics.AppIndex) {
	delete(cb.mods.appglob, appIdx)
}

func (cb *roundCowState) getAppParams(aidx basics.AppIndex) (basics.AppParams, basics.Address, bool, error) {
	modGlobalApp, ok := cb.getModGlobalApp(aidx)
	if ok {
		if modGlobalApp.stateChange == deletedGlobalSC {
			return basics.AppParams{}, basics.Address{}, false, nil
		}
		if modGlobalApp.stateChange == createdGlobalSC {
			return modGlobalApp.params, modGlobalApp.creator, true, nil
		}
	}
	return cb.lookupParent.getAppParams(aidx)
}

func (cb *roundCowState) optedIn(addr basics.Address, appIdx basics.AppIndex) (bool, error) {
	// Check modifiedLocalApp if present for this account, app id
	modLocalApp, ok := cb.getModLocalApp(addr, appIdx)
	if ok {
		if modLocalApp.stateChange == optedOutLocalSC {
			return false, nil
		}
		if modLocalApp.stateChange == optedInLocalSC {
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
	modLocalApp := cb.ensureModLocalApp(addr, appIdx)

	// Clear any existing kv delta, since we should always opt in from a
	// clean state
	modLocalApp.kvCow.clear()

	// Update state accordingly
	modLocalApp.stateChange = optedInLocalSC
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
	modLocalApp := cb.ensureModLocalApp(addr, appIdx)

	// Clear any existing kv delta, since opting out must completely clear
	// the key/value store
	modLocalApp.kvCow.clear()

	// Update state accordingly
	modLocalApp.stateChange = optedOutLocalSC
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
	modGlobalApp := cb.ensureModGlobalApp(appIdx)
	modGlobalApp.stateChange = createdGlobalSC
	modGlobalApp.creator = creator
	modGlobalApp.params = params
	return nil
}

func (cb *roundCowState) updateApp(appIdx basics.AppIndex, approvalProgram, clearStateProgram []byte) error {
	// Ensure app exists
	_, _, exists, err := cb.getAppParams(appIdx)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("cannot updatee app: app %d does not exist", appIdx)
	}

	// Update app
	modGlobalApp := cb.ensureModGlobalApp(appIdx)

	// Copy program bytes, just in case the caller does something with the slice
	approv := make([]byte, len(approvalProgram))
	copy(approv, approvalProgram)

	clear := make([]byte, len(clearStateProgram))
	copy(clear, clearStateProgram)

	modGlobalApp.params.ApprovalProgram = approv
	modGlobalApp.params.ClearStateProgram = clear
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

	// Get/create a modGlobalApp for this app
	modGlobalApp := cb.ensureModGlobalApp(appIdx)

	// Mark app as deleted
	modGlobalApp.stateChange = deletedGlobalSC
	modGlobalApp.creator = basics.Address{}
	modGlobalApp.params = basics.AppParams{}
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
	modLocalApp, ok := cb.getModLocalApp(addr, appIdx)
	if ok {
		// Check kv cow
		hitCow, tv, ok := modLocalApp.kvCow.read(key)
		if hitCow {
			return tv, ok, nil
		}

		// If this delta has opted us in, we should not check our
		// parent, since we were opted out before
		if modLocalApp.stateChange == optedInLocalSC {
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
	modLocalApp := cb.ensureModLocalApp(addr, appIdx)

	// If we are opting in in this cow, don't look up backing values, and
	// always write to the kv cow, since our parent will not have local
	// state for us
	if modLocalApp.stateChange == optedInLocalSC {
		modLocalApp.kvCow.write(key, value, basics.TealValue{}, false)
	} else {
		// Look up backing value (so we don't generate a delta if writing a
		// value equal to the backing value). By this point, we have checked
		// that the user is not opting in to or opting out of the app, so they
		// better already be opted in if the user is making a valid call.
		bv, bvok, err := cb.lookupParent.getLocal(addr, appIdx, key)
		if err != nil {
			return err
		}

		// Write to the cow
		modLocalApp.kvCow.write(key, value, bv, bvok)
	}
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
	modLocalApp := cb.ensureModLocalApp(addr, appIdx)

	// If we are opting in in this cow, don't look up backing values, and
	// tell the kvCow that a backing value is not present, since our parent
	// will not have local state for us
	if modLocalApp.stateChange == optedInLocalSC {
		modLocalApp.kvCow.del(key, false)
	} else {
		// Look up if backing value existed (so we don't generate a delta if
		// there was no entry for the key before)
		_, bvok, err := cb.lookupParent.getLocal(addr, appIdx, key)
		if err != nil {
			return err
		}

		// Write to the cow
		modLocalApp.kvCow.del(key, bvok)
	}
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
	modGlobalApp, ok := cb.getModGlobalApp(appIdx)
	if ok {
		// Check kv cow
		hitCow, tv, ok := modGlobalApp.kvCow.read(key)
		if hitCow {
			return tv, ok, nil
		}

		// If this delta is creating the app, we should not check our
		// parent if the key is missing from the kv cow, since the
		// app did not exist before
		if modGlobalApp.stateChange == createdGlobalSC {
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
	modGlobalApp := cb.ensureModGlobalApp(appIdx)

	// If app is being created, don't look up backing values, and always
	// write to the kv cow, since the app will not exist in our parent
	if modGlobalApp.stateChange == createdGlobalSC {
		modGlobalApp.kvCow.write(key, value, basics.TealValue{}, false)
	} else {
		// Look up backing value (so we don't generate a delta if writing a
		// value equal to the backing value). By this point, we have checked
		// that the app is not being created or deleted in this cow, so it
		// better exist if the user is making a valid call.
		bv, bvok, err := cb.lookupParent.getGlobal(appIdx, key)
		if err != nil {
			return err
		}

		// Write to the cow
		modGlobalApp.kvCow.write(key, value, bv, bvok)
	}
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
	modGlobalApp := cb.ensureModGlobalApp(appIdx)

	// If app is being created, don't look up whether the key exists in
	// parent, and delete any entry for this key in the kv delta (the
	// parent will not have an entry, since the app didn't exist)
	if modGlobalApp.stateChange == createdGlobalSC {
		modGlobalApp.kvCow.del(key, false)
	} else {
		// Look up if backing value existed (so we don't generate a delta if
		// there was no entry for the key before)
		_, bvok, err := cb.lookupParent.getGlobal(appIdx, key)
		if err != nil {
			return err
		}

		// Write to the cow
		modGlobalApp.kvCow.del(key, bvok)
	}
	return nil
}

func (cb *roundCowState) mergeGlobalAppDelta(aidx basics.AppIndex, cga *modifiedGlobalApp) {
	// Grab delta reference for this app id (might be empty)
	pga := cb.ensureModGlobalApp(aidx)

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
The parent modLocalApp can be in one of three states:
	- noLocalSC (indicating no state change)
	- optInLocalSC (indicating we need to opt in)
	- optOutLocalSC (indicating we need to opt out)

The child modLocalApp can also be in one of those three states. There are
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

func (cb *roundCowState) mergeLocalAppDelta(addr basics.Address, aidx basics.AppIndex, cla *modifiedLocalApp) {
	// Grab delta reference for this addr/aidx local state
	pla := cb.ensureModLocalApp(addr, aidx)

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
