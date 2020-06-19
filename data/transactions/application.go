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

package transactions

import (
	"fmt"

	"github.com/algorand/go-algorand/data/basics"
)

const (
	// encodedMaxApplicationArgs sets the allocation bound for the maximum
	// number of ApplicationArgs that a transaction decoded off of the wire
	// can contain. Its value is verified against consensus parameters in
	// TestEncodedAppTxnAllocationBounds
	encodedMaxApplicationArgs = 32

	// encodedMaxAccounts sets the allocation bound for the maximum number
	// of Accounts that a transaction decoded off of the wire can contain.
	// Its value is verified against consensus parameters in
	// TestEncodedAppTxnAllocationBounds
	encodedMaxAccounts = 32

	// encodedMaxForeignApps sets the allocation bound for the maximum
	// number of ForeignApps that a transaction decoded off of the wire can
	// contain. Its value is verified against consensus parameters in
	// TestEncodedAppTxnAllocationBounds
	encodedMaxForeignApps = 32
)

// OnCompletion is an enum representing some layer 1 side effect that an
// ApplicationCall transaction will have if it is included in a block.
//go:generate stringer -type=OnCompletion -output=application_string.go
type OnCompletion uint64

const (
	// NoOpOC indicates that an application transaction will simply call its
	// ApprovalProgram
	NoOpOC OnCompletion = 0

	// OptInOC indicates that an application transaction will allocate some
	// LocalState for the application in the sender's account
	OptInOC OnCompletion = 1

	// CloseOutOC indicates that an application transaction will deallocate
	// some LocalState for the application from the user's account
	CloseOutOC OnCompletion = 2

	// ClearStateOC is similar to CloseOutOC, but may never fail. This
	// allows users to reclaim their minimum balance from an application
	// they no longer wish to opt in to.
	ClearStateOC OnCompletion = 3

	// UpdateApplicationOC indicates that an application transaction will
	// update the ApprovalProgram and ClearStateProgram for the application
	UpdateApplicationOC OnCompletion = 4

	// DeleteApplicationOC indicates that an application transaction will
	// delete the AppParams for the application from the creator's balance
	// record
	DeleteApplicationOC OnCompletion = 5
)

// ApplicationCallTxnFields captures the transaction fields used for all
// interactions with applications
type ApplicationCallTxnFields struct {
	_struct struct{} `codec:",omitempty,omitemptyarray"`

	ApplicationID   basics.AppIndex   `codec:"apid"`
	OnCompletion    OnCompletion      `codec:"apan"`
	ApplicationArgs [][]byte          `codec:"apaa,allocbound=encodedMaxApplicationArgs"`
	Accounts        []basics.Address  `codec:"apat,allocbound=encodedMaxAccounts"`
	ForeignApps     []basics.AppIndex `codec:"apfa,allocbound=encodedMaxForeignApps"`

	LocalStateSchema  basics.StateSchema `codec:"apls"`
	GlobalStateSchema basics.StateSchema `codec:"apgs"`
	ApprovalProgram   []byte             `codec:"apap,allocbound=config.MaxAppProgramLen"`
	ClearStateProgram []byte             `codec:"apsu,allocbound=config.MaxAppProgramLen"`

	// If you add any fields here, remember you MUST modify the Empty
	// method below!
}

// Empty indicates whether or not all the fields in the
// ApplicationCallTxnFields are zeroed out
func (ac *ApplicationCallTxnFields) Empty() bool {
	if ac.ApplicationID != 0 {
		return false
	}
	if ac.OnCompletion != 0 {
		return false
	}
	if ac.ApplicationArgs != nil {
		return false
	}
	if ac.Accounts != nil {
		return false
	}
	if ac.ForeignApps != nil {
		return false
	}
	if ac.LocalStateSchema != (basics.StateSchema{}) {
		return false
	}
	if ac.GlobalStateSchema != (basics.StateSchema{}) {
		return false
	}
	if ac.ApprovalProgram != nil {
		return false
	}
	if ac.ClearStateProgram != nil {
		return false
	}
	return true
}

// AddressByIndex converts an integer index into an address associated with the
// transaction. Index 0 corresponds to the transaction sender, and an index > 0
// corresponds to an offset into txn.Accounts. Returns an error if the index is
// not valid.
func (ac *ApplicationCallTxnFields) AddressByIndex(accountIdx uint64, sender basics.Address) (basics.Address, error) {
	// Index 0 always corresponds to the sender
	if accountIdx == 0 {
		return sender, nil
	}

	// An index > 0 corresponds to an offset into txn.Accounts. Check to
	// make sure the index is valid.
	if accountIdx > uint64(len(ac.Accounts)) {
		err := fmt.Errorf("cannot load account[%d] of %d", accountIdx, len(ac.Accounts))
		return basics.Address{}, err
	}

	// accountIdx must be in [1, len(ac.Accounts)]
	return ac.Accounts[accountIdx-1], nil
}

func (ac *ApplicationCallTxnFields) apply(header Header, balances Balances, spec SpecialAddresses, ad *ApplyData, txnCounter uint64, steva StateEvaluator) (err error) {
	defer func() {
		// If we are returning a non-nil error, then don't return a
		// non-empty EvalDelta. Not required for correctness.
		if err != nil && ad != nil {
			ad.EvalDelta = basics.EvalDelta{}
		}
	}()

	// Sanity check, we should always be passed a non-nil ApplyData
	if ad == nil {
		err = fmt.Errorf("cannot use nil ApplyData")
		return
	}

	// Keep track of the application ID we're working on
	appIdx := ac.ApplicationID
	if appIdx == 0 {
		// We're creating an application, and this will be its ID
		appIdx = basics.AppIndex(txnCounter + 1)
	}

	// Initialize our TEAL evaluation context. Internally, this manages
	// access to balance records for Stateful TEAL programs. Stateful TEAL
	// may only access
	// - The sender's balance record
	// - The balance records of accounts explicitly listed in ac.Accounts
	// - The app creator's balance record (to read/write GlobalState)
	// - The balance records of creators of apps in ac.ForeignApps (to read
	//   GlobalState)
	acctWhitelist := append(ac.Accounts, header.Sender)
	appGlobalWhitelist := append(ac.ForeignApps, appIdx)
	err = steva.InitLedger(balances, acctWhitelist, appGlobalWhitelist, appIdx)
	if err != nil {
		return err
	}

	// Specifying an application ID of 0 indicates application creation
	if ac.ApplicationID == 0 {
		params := basics.AppParams{
			ApprovalProgram:   ac.ApprovalProgram,
			ClearStateProgram: ac.ClearStateProgram,
			LocalStateSchema:  ac.LocalStateSchema,
			GlobalStateSchema: ac.GlobalStateSchema,
		}
		err = steva.CreateApplication(appIdx, header.Sender, params)
		if err != nil {
			return
		}
	}

	// Clear out our LocalState. In this case, we don't execute the
	// ApprovalProgram, since clearing out is always allowed. We only
	// execute the ClearStateProgram, whose failures are ignored.
	if ac.OnCompletion == ClearStateOC {
		pass, evalDelta, err := steva.EvalClearState()
		if err != nil {
			return err
		}
		if pass {
			ad.EvalDelta = evalDelta
		}
		return nil
	}

	// If this is an OptIn transaction, ensure that the sender has
	// LocalState allocated prior to TEAL execution, so that it may be
	// initialized in the same transaction.
	if ac.OnCompletion == OptInOC {
		err = steva.OptInApplication(appIdx, header.Sender)
		if err != nil {
			return err
		}
	}

	// Execute the Approval program
	pass, evalDelta, err := steva.EvalApproval()
	if err != nil {
		return err
	}
	if !pass {
		return fmt.Errorf("transaction rejected by ApprovalProgram")
	}

	switch ac.OnCompletion {
	case NoOpOC:
		// Nothing to do

	case OptInOC:
		// Handled above

	case CloseOutOC:
		err = steva.OptOutApplication(appIdx, header.Sender)
		if err != nil {
			return err
		}

	case DeleteApplicationOC:
		err = steva.DeleteApplication(appIdx)
		if err != nil {
			return err
		}

	case UpdateApplicationOC:
		err = steva.UpdateApplication(appIdx, ac.ApprovalProgram, ac.ClearStateProgram)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid application action")
	}

	// Fill in applyData, so that consumers don't have to implement a
	// stateful TEAL interpreter to apply state changes
	ad.EvalDelta = evalDelta

	return nil
}
