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
	getAppParams(aidx basics.AppIndex) (basics.AppParams, basics.Address, bool, error)
	getCreator(cidx basics.CreatableIndex, ctype basics.CreatableType) (basics.Address, bool, error)
	optedIn(addr basics.Address, appIdx basics.AppIndex) (bool, error)
	getLocal(addr basics.Address, appIdx basics.AppIndex, key string) (basics.TealValue, bool, error)
	getGlobal(appIdx basics.AppIndex, key string) (basics.TealValue, bool, error)
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

func (cb *roundCowState) put(addr basics.Address, old basics.AccountData, new basics.AccountData) {
	prev, present := cb.mods.accts[addr]
	if present {
		cb.mods.accts[addr] = accountDelta{old: prev.old, new: new}
	} else {
		cb.mods.accts[addr] = accountDelta{old: old, new: new}
	}
}

func (cb *roundCowState) putCreatables(addr basics.Address, newCreatables []basics.CreatableLocator, deletedCreatables []basics.CreatableLocator) {
	// Mark creatables as created
	for _, cl := range newCreatables {
		cb.mods.creatables[cl.Index] = modifiedCreatable{
			ctype:   cl.Type,
			creator: addr,
			created: true,
		}
	}

	// Mark creatables as deleted, ensuring we don't produce a delta if the
	// creatable was created in this same cow
	for _, cl := range deletedCreatables {
		_, ok := cb.mods.creatables[cl.Index]
		if ok {
			delete(cb.mods.creatables, cl.Index)
		} else {
			cb.mods.creatables[cl.Index] = modifiedCreatable{
				ctype:   cl.Type,
				creator: addr,
				created: false,
			}
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
therefore nine possible transitions, some of which are valid, and some of which
are invalid.

We use (stateOne, stateTwo) to represent a transition from the parent in
stateOne to the child in stateTwo.

(noLocalSC, noLocalSC)
- We must have been opted in already: just apply k/v deltas

(noLocalSC, optInLocalSC)
- We must have been opted out already: opt in and apply k/v deltas to blank state

(noLocalSC, optOutLocalSC)
- We must have been opted in already: opt out (len k/v deltas must be zero)

(optInLocalSC, noLocalSC)
- Parent opted us in: apply k/v deltas to parent state

(optInLocalSC, optInLocalSC)
- Child must have opted out at some point and opted in again:
	1. Opt out in parent (must succeed), since child must have opted out at
	   some point and this is a convenient way to reset state
	2. Opt in in parent and apply k/v deltas to now blank state

(optInLocalSC, optOutLocalSC)
- Opt out (len k/v deltas must be zero)

(optOutLocalSC, noLocalSC)
- Invalid state. Why did the child produce a delta?

(optOutLocalSC, optInLocalSC)
- Opt in in parent. Apply deltas.

(optOutLocalSC, optOutLocalSC)
- Invalid state. Why did the child produce a delta?

*/

func (cb *roundCowState) mergeLocalAppDelta(addr basics.Address, aidx basics.AppIndex, cla *modifiedLocalApp) {
	// Grab delta reference for this addr/aidx local state
	pla := cb.ensureModLocalApp(addr, aidx)

	// parent optOut -> child optOut is an invalid state, because even if
	// child went through optIn -> optOut, they should have checked that
	// parent was optOut and deleted the delta entirely
	if pla.stateChange == optedOutLocalSC && cla.stateChange == optedOutLocalSC {
		panic("parent cow and child cow both opted out")
	}

	// parent optOut -> child noOp is an invalid state, because the child
	// should not have produced a delta at all
	if pla.stateChange == optedOutLocalSC && cla.stateChange == noLocalSC {
		panic("child cow produced noop delta after parent opted out")
	}

	switch cla.stateChange {
	case optedOutLocalSC:
		// Sanity check: opting out, child should have no kv delta
		if len(cla.kvCow.delta) != 0 {
			panic("child opted out, but child kvCow had nonzero length delta")
		}

		// If the child opted out, replay that event
		err := cb.optOut(aidx, addr)
		if err != nil {
			panic(fmt.Sprintf("unable to merge opted out app to parent: %v", err))
		}
	case optedInLocalSC:
		if pla.stateChange == optedInLocalSC {
			// If parent also opted in, then we must first opt out in parent to let
			// the child's opt in take precedence
			err := cb.optOut(aidx, addr)
			if err != nil {
				panic(fmt.Sprintf("failed to opt out in preparation of child opt in: %v", err))
			}
		}

		// If the child opted in, replay that event
		err := cb.optIn(aidx, addr)
		if err != nil {
			panic(fmt.Sprintf("unable to merge opted-in app to parent: %v", err))
		}

		// TODO: make unit test
		if len(pla.kvCow.delta) != 0 {
			panic(fmt.Sprintf("child opted in, but parent had nonzero kv delta immediately after opt in"))
		}
	case noLocalSC:
	default:
		panic(fmt.Sprintf("unknown localAppStateChange %v", cla.stateChange))
	}

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

	for aidx, modGlobalApp := range cb.mods.appglob {
		cb.commitParent.mergeGlobalAppDelta(aidx, modGlobalApp)
	}

	for addr, modLocalApps := range cb.mods.appaccts {
		for aidx, modLocalApp := range modLocalApps {
			cb.commitParent.mergeLocalAppDelta(addr, aidx, modLocalApp)
		}
	}

	for txid, lv := range cb.mods.Txids {
		cb.commitParent.mods.Txids[txid] = lv
	}

	for txl, expires := range cb.mods.txleases {
		cb.commitParent.mods.txleases[txl] = expires
	}

	for cidx, cdelta := range cb.mods.creatables {
		pdelta, ok := cb.commitParent.mods.creatables[cidx]
		if ok {
			// If the parent created the creatable, and the child deleted it,
			// then we can avoid creating a delta entirely.
			if pdelta.created && !cdelta.created {
				delete(cb.commitParent.mods.creatables, cidx)
				continue
			}
		}

		// Otherwise, all creatable modifications should propogate to the parent
		cb.commitParent.mods.creatables[cidx] = cdelta
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
