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

func (cb *roundCowState) getAppCreator(aidx basics.AppIndex) (basics.Address, bool, error) {
	modGlobalApp, ok := cb.getModGlobalApp(aidx)
	if ok {
		if modGlobalApp.stateChange == deletedGlobalSC {
			return basics.Address{}, false, nil
		}
		if modGlobalApp.stateChange == createdGlobalSC {
			return modGlobalApp.creator, true, nil
		}
	}
	return cb.lookupParent.getAppCreator(aidx)
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

func (cb *roundCowState) optIn(addr basics.Address, appIdx basics.AppIndex) error {
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

	// Clear any existing kv delta, since we should always be optin in
	// from a clean state
	modLocalApp.kvCow.clear()

	// Update state accordingly
	modLocalApp.stateChange = optedInLocalSC
	return nil
}

func (cb *roundCowState) optOut(addr basics.Address, appIdx basics.AppIndex) error {
	// Make sure we're opted in
	optedIn, err := cb.optedIn(addr, appIdx)
	if err != nil {
		return err
	}
	if !optedIn {
		return fmt.Errorf("cannot opt out: acct %v is not opted in to app %d", addr, appIdx)
	}

	// If we were opted out in the parent, delete our modLocalApp entirely,
	// and short circuit, because the net change for this local state must
	// be zero.
	poptedIn, err := cb.lookupParent.optedIn(addr, appIdx)
	if err != nil {
		return err
	}
	if !poptedIn {
		cb.deleteModLocalApp(addr, appIdx)
		return nil
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

func (cb *roundCowState) createApp(appIdx basics.AppIndex, creator basics.Address) error {
	// Ensure app does not exist
	_, exists, err := cb.getAppCreator(appIdx)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("cannot crate app: app %d already exists", appIdx)
	}

	// Mark app as created
	modGlobalApp := cb.ensureModGlobalApp(appIdx)
	modGlobalApp.stateChange = createdGlobalSC
	modGlobalApp.creator = creator
	return nil
}

func (cb *roundCowState) deleteApp(appIdx basics.AppIndex) error {
	// Ensure app exists
	_, exists, err := cb.getAppCreator(appIdx)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("cannot delete app: app %d does not exist", appIdx)
	}

	// Get/create a modGlobalApp for this app
	modGlobalApp := cb.ensureModGlobalApp(appIdx)

	// If we created the app in this cow, just delete the modGlobalApp
	// entirely. Otherwise, mark the app as deleted.
	if modGlobalApp.stateChange == createdGlobalSC {
		cb.deleteModGlobalApp(appIdx)
		return nil
	}

	// Mark app as deleted
	modGlobalApp.stateChange = deletedGlobalSC
	modGlobalApp.creator = basics.Address{}
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
	_, exists, err := cb.getAppCreator(appIdx)
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
	_, exists, err := cb.getAppCreator(appIdx)
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
	_, exists, err := cb.getAppCreator(appIdx)
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
