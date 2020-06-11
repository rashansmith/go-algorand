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
	appaccts map[basics.Address]modifiedLocalApps

	// modified global application data (incl. programs + global state)
	apps map[basics.AppIndex]modifiedGlobalApp

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
			Txids:      make(map[transactions.Txid]basics.Round),
			txleases:   make(map[txlease]basics.Round),
			creatables: make(map[basics.CreatableIndex]modifiedCreatable),
			hdr:        cb.mods.hdr,
		},
	}
}

func (cb *roundCowState) optedIn(addr basics.Address, appIdx basics.AppIndex) (bool, error) {
	// Have we modified any local app state for this account?
	modLocalApps, ok := cb.mods.appaccts[addr]
	if ok {
		// Within this account, have we modified any local apps tate for this app?
		modLocalApp, ok := modLocalApps[appIdx]
		if ok {
			// If we've opted out, return false
			if modLocalApp.optedOut {
				return false, nil
			}
		}
	}
	return cb.lookupParent.optedIn(addr, appIdx)
}

func (x *roundCowState) getLocal(addr basics.Address, appIdx basics.AppIndex, key string) (basics.TealValue, bool, error) {
	return basics.TealValue{}, false, nil
}

func (x *roundCowState) setLocal(addr basics.Address, appIdx basics.AppIndex, key string, value basics.TealValue) error {
	return nil
}

func (x *roundCowState) delLocal(addr basics.Address, appIdx basics.AppIndex, key string) error {
	return nil
}

func (x *roundCowState) getGlobal(appIdx basics.AppIndex, key string) (basics.TealValue, bool, error) {
	return basics.TealValue{}, false, nil
}

func (x *roundCowState) setGlobal(appIdx basics.AppIndex, key string, value basics.TealValue) error {
	return nil
}

func (x *roundCowState) delGlobal(appIdx basics.AppIndex, key string) error {
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
