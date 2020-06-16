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
	"errors"
	"fmt"

	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/data/transactions/logic"
)

// TODO remove round from balances

// AppTealGlobals contains data accessible by the "global" opcode.
type AppTealGlobals struct {
	CurrentRound    basics.Round
	LatestTimestamp int64
}

// appTealEvaluator implements transactions.StateEvaluator. When applying an
// ApplicationCall transaction, InitLedger is called, followed by Check and/or
// Eval. These pass the initialized LedgerForLogic (appLedger) to the TEAL
// interpreter.
type appTealEvaluator struct {
	evalParams     logic.EvalParams
	basecow        *roundCowState
	ledgerTemplate *appLedger
	AppTealGlobals
}

// appLedger implements logic.LedgerForLogic
type appLedger struct {
	addresses map[basics.Address]bool
	apps      map[basics.AppIndex]bool
	cow       *roundCowState
	appIdx    basics.AppIndex
	AppTealGlobals
}

type programType uint64

const (
	approvalProgram   programType = 0
	clearStateProgram programType = 1
)

func (ae *appTealEvaluator) evalProgram(ptype programType) (pass bool, stateDelta basics.EvalDelta, err error) {
	defer func() {
		// Clear out the TEAL ledger and ledger template cow when we're
		// done to make sure they cannot be reused by mistake and so
		// the child cow can be garbage collected
		ae.evalParams.Ledger = nil
		if ae.ledgerTemplate != nil {
			ae.ledgerTemplate.cow = nil
		}
	}()

	// Sanity check that we were initialized properly
	if ae.basecow == nil {
		err = fmt.Errorf("appTealEvaluator EvalApproval called before initialization")
		return
	}
	if ae.ledgerTemplate == nil {
		err = fmt.Errorf("appTealEvaluator EvalApproval called before ledger template initialized")
		return
	}

	// Create a child cow to be reverted if TEAL execution fails
	child := ae.basecow.child()

	// Initialize ledger for TEAL evaluator
	ae.ledgerTemplate.cow = child
	ae.evalParams.Ledger = ae.ledgerTemplate

	// Fetch the relevant application parameters
	params, _, ok, err := child.getAppParams(ae.ledgerTemplate.appIdx)
	if err != nil {
		return
	}
	if !ok {
		err = fmt.Errorf("application %v does not exist", ae.ledgerTemplate.appIdx)
	}

	// Select the program bytes to execute
	var program []byte
	switch ptype {
	case approvalProgram:
		program = params.ApprovalProgram
	case clearStateProgram:
		program = params.ClearStateProgram
	default:
		panic(fmt.Sprintf("unknown program type: %v", ptype))
	}

	// Run the approval program
	pass, stateDelta, err = logic.EvalStateful(program, ae.evalParams)

	// If it succeeded, commit cow to parent
	if err == nil && pass {
		child.commitToParent()
	}

	return pass, stateDelta, err
}

// EvalApproval evaluates the approval program for an application, applying the
// results if the program succeeded
func (ae *appTealEvaluator) EvalApproval() (pass bool, stateDelta basics.EvalDelta, err error) {
	return ae.evalProgram(approvalProgram)
}

// EvalClearState evaluates the clear state program for an application,
// applying the results if the program succeeded
func (ae *appTealEvaluator) EvalClearState() (pass bool, stateDelta basics.EvalDelta, err error) {
	return ae.evalProgram(clearStateProgram)
}

func (ae *appTealEvaluator) CreateApplication(appIdx basics.AppIndex, creator basics.Address, params basics.AppParams) error {
	return ae.basecow.createApp(appIdx, creator, params)
}

func (ae *appTealEvaluator) UpdateApplication(appIdx basics.AppIndex, approvalProgram, clearStateProgram []byte) error {
	return ae.basecow.updateApp(appIdx, approvalProgram, clearStateProgram)
}

func (ae *appTealEvaluator) DeleteApplication(appIdx basics.AppIndex) error {
	return ae.basecow.deleteApp(appIdx)
}

func (ae *appTealEvaluator) OptInApplication(appIdx basics.AppIndex, addr basics.Address) error {
	return ae.basecow.optIn(appIdx, addr)
}

func (ae *appTealEvaluator) OptOutApplication(appIdx basics.AppIndex, addr basics.Address) error {
	return ae.basecow.optIn(appIdx, addr)
}

// InitLedger initializes an appLedger, which satisfies the
// logic.LedgerForLogic interface. The acctWhitelist lists all the accounts
// whose balance records we can fetch information like LocalState and balance
// from, and the appGlobalWhitelist lists all the app IDs we are allowed to
// fetch global state for (which requires looking up the creator's balance
// record).
//
// InitLedger must be called before calling EvalApproval or EvalClearState

func (ae *appTealEvaluator) InitLedger(balances transactions.Balances, acctWhitelist []basics.Address, appGlobalWhitelist []basics.AppIndex, appIdx basics.AppIndex) (err error) {
	// TODO: kill this by either extending the balances interface or making a new one
	cow, ok := balances.(*roundCowState)
	if !ok {
		return errors.New("InitLedger was passed an unexpected implementation of transactions.Balances")
	}

	// Store the base cow so that we can pass a child cow to the TEAL
	// interpreter during eval
	ae.basecow = cow

	// Fill in our app ledger with everything except a cow, which we'll add at Eval-time
	ae.ledgerTemplate, err = newAppLedger(acctWhitelist, appGlobalWhitelist, appIdx, ae.AppTealGlobals)
	if err != nil {
		return err
	}
	return nil
}

func newAppLedger(acctWhitelist []basics.Address, appGlobalWhitelist []basics.AppIndex, appIdx basics.AppIndex, globals AppTealGlobals) (al *appLedger, err error) {
	if len(acctWhitelist) < 1 {
		err = fmt.Errorf("appLedger acct whitelist should at least include txn sender")
		return
	}

	if len(appGlobalWhitelist) < 1 {
		err = fmt.Errorf("appLedger app whitelist should at least include this appIdx")
		return
	}

	if appIdx == 0 {
		err = fmt.Errorf("cannot create appLedger for appIdx 0")
		return
	}

	al = &appLedger{}
	al.appIdx = appIdx
	al.addresses = make(map[basics.Address]bool, len(acctWhitelist))
	al.apps = make(map[basics.AppIndex]bool, len(appGlobalWhitelist))
	al.AppTealGlobals = globals

	for _, addr := range acctWhitelist {
		al.addresses[addr] = true
	}

	for _, aidx := range appGlobalWhitelist {
		al.apps[aidx] = true
	}

	return al, nil
}

// MakeDebugAppLedger returns logic.LedgerForLogic suitable for debug or dryrun
func MakeDebugAppLedger(balances transactions.Balances, acctWhitelist []basics.Address, appGlobalWhitelist []basics.AppIndex, appIdx basics.AppIndex, globals AppTealGlobals) (al logic.LedgerForLogic, err error) {
	// TODO: kill this by either extending the balances interface or making a new one
	cow, ok := balances.(*roundCowState)
	if !ok {
		return nil, errors.New("MakeDebugAppLedger was passed an unexpected implementation of transactions.Balances")
	}

	appLedger, err := newAppLedger(acctWhitelist, appGlobalWhitelist, appIdx, globals)
	if err != nil {
		return nil, err
	}
	appLedger.cow = cow

	return appLedger, nil
}

func (al *appLedger) Round() basics.Round {
	return al.AppTealGlobals.CurrentRound
}

func (al *appLedger) LatestTimestamp() int64 {
	return al.AppTealGlobals.LatestTimestamp
}

func (al *appLedger) ApplicationID() basics.AppIndex {
	return al.appIdx
}

func (al *appLedger) Balance(addr basics.Address) (res basics.MicroAlgos, err error) {
	// Ensure requested address is on whitelist
	if !al.addresses[addr] {
		err = fmt.Errorf("cannot access balance for %s, not sender or in txn.Addresses", addr.String())
		return
	}

	// Fetch record with pending rewards applied
	record, err := al.cow.Get(addr, true)
	if err != nil {
		return
	}

	return record.MicroAlgos, nil
}

func (al *appLedger) AssetHolding(addr basics.Address, assetIdx basics.AssetIndex) (holding basics.AssetHolding, err error) {
	// Ensure requested address is on whitelist
	if !al.addresses[addr] {
		err = fmt.Errorf("cannot access asset holding for %s, not sender or in txn.Addresses", addr.String())
		return
	}

	// Fetch the requested balance record
	record, err := al.cow.Get(addr, false)
	if err != nil {
		return
	}

	// Ensure we have the requested holding
	holding, ok := record.Assets[assetIdx]
	if !ok {
		err = fmt.Errorf("account %s has not opted in to asset %d", addr.String(), assetIdx)
		return
	}

	return holding, nil
}

func (al *appLedger) AssetParams(addr basics.Address, assetIdx basics.AssetIndex) (params basics.AssetParams, err error) {
	// Ensure requested address is on whitelist
	if !al.addresses[addr] {
		err = fmt.Errorf("cannot access asset params for %s, not sender or in txn.Addresses", addr.String())
		return
	}

	// Fetch the requested balance record
	record, err := al.cow.Get(addr, false)
	if err != nil {
		return
	}

	// Ensure account created the requested asset
	params, ok := record.AssetParams[assetIdx]
	if !ok {
		err = fmt.Errorf("account %s has not created asset %d", addr.String(), assetIdx)
		return
	}

	return params, nil
}

func (al *appLedger) OptedIn(addr basics.Address, appIdx basics.AppIndex) (bool, error) {
	return false, nil
}

func (al *appLedger) GetLocal(addr basics.Address, appIdx basics.AppIndex, key string) (basics.TealValue, bool, error) {
	return basics.TealValue{}, false, nil
}

func (al *appLedger) SetLocal(addr basics.Address, appIdx basics.AppIndex, key string, value basics.TealValue) error {
	return nil
}

func (al *appLedger) DelLocal(addr basics.Address, appIdx basics.AppIndex, key string) error {
	return nil
}

func (al *appLedger) GetGlobal(appIdx basics.AppIndex, key string) (basics.TealValue, bool, error) {
	return basics.TealValue{}, false, nil
}

func (al *appLedger) SetGlobal(appIdx basics.AppIndex, key string, value basics.TealValue) error {
	return nil
}

func (al *appLedger) DelGlobal(appIdx basics.AppIndex, key string) error {
	return nil
}
