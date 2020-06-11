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

	"github.com/algorand/go-algorand/config"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
)

//   ___________________
// < cow = Copy On Write >
//   -------------------
//          \   ^__^
//           \  (oo)\_______
//              (__)\       )\/\
//                  ||----w |
//                  ||     ||

type roundCowParent interface {
	lookup(basics.Address) (basics.AccountData, error)
	isDup(basics.Round, basics.Round, transactions.Txid, txlease) (bool, error)
	txnCounter() uint64
	getAssetCreator(aidx basics.AssetIndex) (basics.Address, bool, error)
	getAppCreator(aidx basics.AppIndex) (basics.Address, bool, error)
	getCreator(cidx basics.CreatableIndex, ctype basics.CreatableType) (basics.Address, bool, error)

	optedIn(addr basics.Address, appIdx basics.AppIndex) (bool, error)
	getLocal(addr basics.Address, appIdx basics.AppIndex, key string) (basics.TealValue, bool, error)
	setLocal(addr basics.Address, appIdx basics.AppIndex, key string, value basics.TealValue) error
	delLocal(addr basics.Address, appIdx basics.AppIndex, key string) error
	getGlobal(appIdx basics.AppIndex, key string) (basics.TealValue, bool, error)
	setGlobal(appIdx basics.AppIndex, key string, value basics.TealValue) error
	delGlobal(appIdx basics.AppIndex, key string) error
}

type roundCowState struct {
	lookupParent roundCowParent
	commitParent *roundCowState
	proto        config.ConsensusParams
	mods         StateDelta
}

// StateDelta describes the delta between a given round to the previous round
type StateDelta struct {
	// modified accounts
	accts map[basics.Address]accountDelta

	// modified local application data (local key/value stores)
	appaccts map[basics.Address]map[basics.AppIndex]*modifiedLocalApp

	// modified global application data (incl. programs + global state)
	appglob map[basics.AppIndex]*modifiedGlobalApp

	// new Txids for the txtail and TxnCounter, mapped to txn.LastValid
	Txids map[transactions.Txid]basics.Round

	// new txleases for the txtail mapped to expiration
	txleases map[txlease]basics.Round

	// new creatables creator lookup table
	creatables map[basics.CreatableIndex]modifiedCreatable

	// new block header; read-only
	hdr *bookkeeping.BlockHeader
}

func makeRoundCowState(b roundCowParent, hdr bookkeeping.BlockHeader) *roundCowState {
	return &roundCowState{
		lookupParent: b,
		commitParent: nil,
		proto:        config.Consensus[hdr.CurrentProtocol],
		mods: StateDelta{
			accts:      make(map[basics.Address]accountDelta),
			appaccts:   make(map[basics.Address]map[basics.AppIndex]*modifiedLocalApp),
			appglob:    make(map[basics.AppIndex]*modifiedGlobalApp),
			Txids:      make(map[transactions.Txid]basics.Round),
			txleases:   make(map[txlease]basics.Round),
			creatables: make(map[basics.CreatableIndex]modifiedCreatable),
			hdr:        &hdr,
		},
	}
}

func (cb *roundCowState) rewardsLevel() uint64 {
	return cb.mods.hdr.RewardsLevel
}

func (cb *roundCowState) getCreator(cidx basics.CreatableIndex, ctype basics.CreatableType) (creator basics.Address, ok bool, err error) {
	delta, ok := cb.mods.creatables[cidx]
	if ok {
		if delta.created && delta.ctype == ctype {
			return delta.creator, true, nil
		}
		return basics.Address{}, false, nil
	}
	return cb.lookupParent.getCreator(cidx, ctype)
}

func (cb *roundCowState) getAppCreator(aidx basics.AppIndex) (basics.Address, bool, error) {
	return cb.getCreator(basics.CreatableIndex(aidx), basics.AppCreatable)
}

func (cb *roundCowState) getAssetCreator(aidx basics.AssetIndex) (basics.Address, bool, error) {
	return cb.getCreator(basics.CreatableIndex(aidx), basics.AssetCreatable)
}

func (cb *roundCowState) lookup(addr basics.Address) (data basics.AccountData, err error) {
	d, ok := cb.mods.accts[addr]
	if ok {
		return d.new, nil
	}

	return cb.lookupParent.lookup(addr)
}

func (cb *roundCowState) isDup(firstValid, lastValid basics.Round, txid transactions.Txid, txl txlease) (bool, error) {
	_, present := cb.mods.Txids[txid]
	if present {
		return true, nil
	}

	if cb.proto.SupportTransactionLeases && (txl.lease != [32]byte{}) {
		expires, ok := cb.mods.txleases[txl]
		if ok && cb.mods.hdr.Round <= expires {
			return true, nil
		}
	}

	return cb.lookupParent.isDup(firstValid, lastValid, txid, txl)
}

func (cb *roundCowState) txnCounter() uint64 {
	return cb.lookupParent.txnCounter() + uint64(len(cb.mods.Txids))
}

func (cb *roundCowState) put(addr basics.Address, old basics.AccountData, new basics.AccountData, newCreatables []basics.CreatableLocator, deletedCreatables []basics.CreatableLocator) {
	prev, present := cb.mods.accts[addr]
	if present {
		cb.mods.accts[addr] = accountDelta{old: prev.old, new: new}
	} else {
		cb.mods.accts[addr] = accountDelta{old: old, new: new}
	}

	for _, cl := range newCreatables {
		cb.mods.creatables[cl.Index] = modifiedCreatable{
			ctype:   cl.Type,
			creator: addr,
			created: true,
		}
	}

	for _, cl := range deletedCreatables {
		cb.mods.creatables[cl.Index] = modifiedCreatable{
			ctype:   cl.Type,
			creator: addr,
			created: false,
		}
	}
}

func (cb *roundCowState) addTx(txn transactions.Transaction, txid transactions.Txid) {
	cb.mods.Txids[txid] = txn.LastValid
	cb.mods.txleases[txlease{sender: txn.Sender, lease: txn.Lease}] = txn.LastValid
}

func (cb *roundCowState) child() *roundCowState {
	return &roundCowState{
		lookupParent: cb,
		commitParent: cb,
		proto:        cb.proto,
		mods: StateDelta{
			accts:      make(map[basics.Address]accountDelta),
			appaccts:   make(map[basics.Address]map[basics.AppIndex]*modifiedLocalApp),
			appglob:    make(map[basics.AppIndex]*modifiedGlobalApp),
			Txids:      make(map[transactions.Txid]basics.Round),
			txleases:   make(map[txlease]basics.Round),
			creatables: make(map[basics.CreatableIndex]modifiedCreatable),
			hdr:        cb.mods.hdr,
		},
	}
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

func (cb *roundCowState) makeModLocalApp(addr basics.Address, appIdx basics.AppIndex) (*modifiedLocalApp, error) {
	// Have we already modified any local app state for this account?
	modLocalApps, ok := cb.mods.appaccts[addr]
	if !ok {
		modLocalApps = make(map[basics.AppIndex]*modifiedLocalApp, 1)
		cb.mods.appaccts[addr] = modLocalApps
	}

	// Ensure we haven't already initialized modified state for this app
	// for this account
	modLocalApp, ok := modLocalApps[appIdx]
	if ok {
		return nil, fmt.Errorf("should not have mods for app %d on acct %v", appIdx, addr)
	}

	// Initialize a modifiedLocalApp to track any future changes
	modLocalApp = &modifiedLocalApp{
		kvCow: makeKeyValueCow(),
	}
	modLocalApps[appIdx] = modLocalApp

	return modLocalApp, nil
}

func (cb *roundCowState) getModGlobalApp(appIdx basics.AppIndex) (*modifiedGlobalApp, bool) {
	modGlobalApp, ok := cb.mods.appglob[appIdx]
	return modGlobalApp, ok
}

func (cb *roundCowState) makeModGlobalApp(appIdx basics.AppIndex) (*modifiedGlobalApp, error) {
	// Ensure we haven't already initialized modified state for this app
	modGlobalApp, ok := cb.mods.appglob[appIdx]
	if ok {
		return nil, fmt.Errorf("should not have global mods for app %d", appIdx)
	}

	modGlobalApp = &modifiedGlobalApp{
		kvCow: makeKeyValueCow(),
	}
	cb.mods.appglob[appIdx] = modGlobalApp

	return modGlobalApp, nil
}

func (cb *roundCowState) optedIn(addr basics.Address, appIdx basics.AppIndex) (bool, error) {
	// Check kv cow if present
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

func (cb *roundCowState) getLocal(addr basics.Address, appIdx basics.AppIndex, key string) (basics.TealValue, bool, error) {
	// Check if we have a local kv cow
	modLocalApp, ok := cb.getModLocalApp(addr, appIdx)
	if ok {
		// Ensure account has not opted out
		if modLocalApp.stateChange == optedOutLocalSC {
			err := fmt.Errorf("cannot get local state, %v not opted in to app %d", addr, appIdx)
			return basics.TealValue{}, false, err
		}

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
	// Ensure we have a kv cow
	modLocalApp, ok := cb.getModLocalApp(addr, appIdx)
	if !ok {
		modLocalApp, err = cb.makeModLocalApp(addr, appIdx)
		if err != nil {
			return err
		}
	}

	// Ensure we have not opted out
	if modLocalApp.stateChange == optedOutLocalSC {
		err = fmt.Errorf("cannot write local key: acct %s is not opted in to app %d", addr, appIdx)
		return err
	}

	// If we are opting in, don't look up backing values, and always write
	// to the kv cow, since our parent will not have local state for us
	if modLocalApp.stateChange == optedInLocalSC {
		modLocalApp.kvCow.write(key, value, basics.TealValue{}, false)
		return nil
	}

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
	return nil
}

func (cb *roundCowState) delLocal(addr basics.Address, appIdx basics.AppIndex, key string) (err error) {
	// Ensure we have a kv cow
	modLocalApp, ok := cb.getModLocalApp(addr, appIdx)
	if !ok {
		modLocalApp, err = cb.makeModLocalApp(addr, appIdx)
		if err != nil {
			return err
		}
	}

	// Ensure we have not opted out
	if modLocalApp.stateChange == optedOutLocalSC {
		err = fmt.Errorf("cannot delete local key: acct %s is not opted in to app %d", addr, appIdx)
		return err
	}

	// If we are opting in, don't look up backing values, and always write
	// to the kv cow, since our parent will not have local state for us
	if modLocalApp.stateChange == optedInLocalSC {
		modLocalApp.kvCow.del(key, false)
		return nil
	}

	// Look up if backing value existed (so we don't generate a delta if
	// there was no entry for the key before)
	_, bvok, err := cb.lookupParent.getLocal(addr, appIdx, key)
	if err != nil {
		return err
	}

	// Write to the cow
	modLocalApp.kvCow.del(key, bvok)
	return nil
}

func (cb *roundCowState) getGlobal(appIdx basics.AppIndex, key string) (basics.TealValue, bool, error) {
	// Check if we have global modified app info/kv cow
	modGlobalApp, ok := cb.getModGlobalApp(appIdx)
	if ok {
		// Ensure app has not been deleted
		if modGlobalApp.stateChange == deletedGlobalSC {
			err := fmt.Errorf("cannot read global key: app %d does not exist", appIdx)
			return basics.TealValue{}, false, err
		}

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
	// Ensure we have a kv cow
	modGlobalApp, ok := cb.getModGlobalApp(appIdx)
	if !ok {
		modGlobalApp, err = cb.makeModGlobalApp(appIdx)
		if err != nil {
			return err
		}
	}

	// Ensure app has not been deleted
	if modGlobalApp.stateChange == deletedGlobalSC {
		err = fmt.Errorf("cannot write global key: app %d does not exist", appIdx)
		return err
	}

	// If app is being created, don't look up backing values, and always
	// write to the kv cow, since the app will not exist in our parent
	if modGlobalApp.stateChange == createdGlobalSC {
		modGlobalApp.kvCow.write(key, value, basics.TealValue{}, false)
		return nil
	}

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
	return nil
}

func (cb *roundCowState) delGlobal(appIdx basics.AppIndex, key string) (err error) {
	// Ensure we have a kv cow
	modGlobalApp, ok := cb.getModGlobalApp(appIdx)
	if !ok {
		modGlobalApp, err = cb.makeModGlobalApp(appIdx)
		if err != nil {
			return err
		}
	}

	// Ensure app has not been deleted
	if modGlobalApp.stateChange == deletedGlobalSC {
		err = fmt.Errorf("cannot delete global key: app %d does not exist", appIdx)
		return err
	}

	// If app is being created, don't look up whether the key exists in
	// parent, and delete any entry for this key in the kv delta (the
	// parent will not have an entry, since the app didn't exist)
	if modGlobalApp.stateChange == createdGlobalSC {
		modGlobalApp.kvCow.del(key, false)
		return nil
	}

	// Look up if backing value existed (so we don't generate a delta if
	// there was no entry for the key before)
	_, bvok, err := cb.lookupParent.getGlobal(appIdx, key)
	if err != nil {
		return err
	}

	// Write to the cow
	modGlobalApp.kvCow.del(key, bvok)
	return nil
}

func (cb *roundCowState) commitToParent() {
	for addr, delta := range cb.mods.accts {
		prev, present := cb.commitParent.mods.accts[addr]
		if present {
			cb.commitParent.mods.accts[addr] = accountDelta{
				old: prev.old,
				new: delta.new,
			}
		} else {
			cb.commitParent.mods.accts[addr] = delta
		}
	}

	// TODO(app refactor) merge app local/global deltas

	for txid, lv := range cb.mods.Txids {
		cb.commitParent.mods.Txids[txid] = lv
	}
	for txl, expires := range cb.mods.txleases {
		cb.commitParent.mods.txleases[txl] = expires
	}
	for cidx, delta := range cb.mods.creatables {
		cb.commitParent.mods.creatables[cidx] = delta
	}
}

func (cb *roundCowState) modifiedAccounts() []basics.Address {
	res := make([]basics.Address, len(cb.mods.accts))
	i := 0
	for addr := range cb.mods.accts {
		res[i] = addr
		i++
	}
	return res
}
