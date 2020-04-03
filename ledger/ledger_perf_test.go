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
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/algorand/go-deadlock"
	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand/agreement"
	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/data/basics"
	"github.com/algorand/go-algorand/data/bookkeeping"
	"github.com/algorand/go-algorand/data/transactions"
	"github.com/algorand/go-algorand/data/transactions/verify"
	"github.com/algorand/go-algorand/logging"
	"github.com/algorand/go-algorand/protocol"
)

type testParams struct {
	txType     string
	name       string
	program    string
	schemaSize uint64
}

var testCases map[string]testParams

func makeUnsignedPaymentTx(sender basics.Address, round int) transactions.Transaction {
	return transactions.Transaction{
		Type: protocol.PaymentTx,
		Header: transactions.Header{
			FirstValid: basics.Round(round),
			LastValid:  basics.Round(round + 1000),
			Fee:        basics.MicroAlgos{Raw: 1000},
		},
		PaymentTxnFields: transactions.PaymentTxnFields{
			Receiver: sender,
			Amount:   basics.MicroAlgos{Raw: 1234},
		},
	}
}

type alwaysVerifiedCache struct{}

func (vc *alwaysVerifiedCache) Verified(txn transactions.SignedTxn, params verify.Params) bool {
	return true
}

func benchmarkFullBlocks(params testParams, b *testing.B) {
	dbTempDir, err := ioutil.TempDir("", "testdir"+b.Name())
	require.NoError(b, err)
	dbName := fmt.Sprintf("%s.%d", b.Name(), crypto.RandUint64())
	dbPrefix := filepath.Join(dbTempDir, dbName)
	defer os.RemoveAll(dbTempDir)

	genesisInitState := getInitState()

	// Use future protocol
	genesisInitState.Block.BlockHeader.GenesisHash = crypto.Digest{}
	genesisInitState.Block.CurrentProtocol = protocol.ConsensusFuture
	genesisInitState.GenesisHash = crypto.Digest{1}
	genesisInitState.Block.BlockHeader.GenesisHash = crypto.Digest{1}

	creator := basics.Address{}
	_, err = rand.Read(creator[:])
	require.NoError(b, err)
	genesisInitState.Accounts[creator] = basics.MakeAccountData(basics.Offline, basics.MicroAlgos{Raw: 1234567890})

	// open first ledger
	const inMem = false // use persistent storage
	const archival = true
	l0, err := OpenLedger(logging.Base(), dbPrefix, inMem, genesisInitState, archival)
	require.NoError(b, err)

	// open second ledger
	dbName = fmt.Sprintf("%s.%d.2", b.Name(), crypto.RandUint64())
	dbPrefix = filepath.Join(dbTempDir, dbName)
	l1, err := OpenLedger(logging.Base(), dbPrefix, inMem, genesisInitState, archival)
	require.NoError(b, err)

	blk := genesisInitState.Block

	numBlocks := b.N
	cert := agreement.Certificate{}
	var blocks []bookkeeping.Block
	var txPerBlock int
	for i := 0; i < numBlocks+2; i++ {
		blk.BlockHeader.Round++
		blk.BlockHeader.TimeStamp += int64(crypto.RandUint64() % 100 * 1000)
		blk.BlockHeader.GenesisID = "x"

		// If this is the first block, add a blank one to both ledgers
		if i == 0 {
			err = l0.AddBlock(blk, cert)
			require.NoError(b, err)
			err = l1.AddBlock(blk, cert)
			require.NoError(b, err)
			continue
		}

		// Construct evaluator for next block
		prev, err := l0.BlockHdr(basics.Round(i))
		require.NoError(b, err)
		newBlk := bookkeeping.MakeBlock(prev)
		eval, err := l0.StartEvaluator(newBlk.BlockHeader)
		require.NoError(b, err)

		// build a payset
		var j int
		for {
			j++
			// make a transaction of the appropriate type
			var tx transactions.Transaction
			switch params.txType {
			case "pay":
				tx = makeUnsignedPaymentTx(creator, i)
			default:
				panic("unknown tx type")
			}

			tx.Sender = creator
			tx.Note = []byte(fmt.Sprintf("%d,%d", i, j))
			tx.GenesisHash = crypto.Digest{1}

			// add tx to block
			var stxn transactions.SignedTxn
			stxn.Txn = tx
			err = eval.Transaction(stxn, transactions.ApplyData{})

			// check if block is full
			if err == ErrNoSpace {
				txPerBlock = len(eval.block.Payset)
				break
			} else {
				require.NoError(b, err)
			}

			// First block just creates app
			if i == 1 {
				break
			}
		}

		lvb, err := eval.GenerateBlock()
		require.NoError(b, err)

		// If this is the app creation block, add to both ledgers
		if i == 1 {
			err = l0.AddBlock(lvb.blk, cert)
			require.NoError(b, err)
			err = l1.AddBlock(lvb.blk, cert)
			require.NoError(b, err)
			continue
		}

		// For all other blocks, add just to the first ledger, and stash
		// away to be replayed in the second ledger while running timer
		err = l0.AddBlock(lvb.blk, cert)
		require.NoError(b, err)

		blocks = append(blocks, lvb.blk)
	}

	b.Logf("built %d blocks, each with %d txns", numBlocks, txPerBlock)

	// eval + add all the (valid) blocks to the second ledger, measuring it this time
	vc := alwaysVerifiedCache{}
	b.ResetTimer()
	for _, blk := range blocks {
		_, err = l1.eval(context.Background(), blk, true, &vc, nil)
		require.NoError(b, err)
		err = l1.AddBlock(blk, cert)
		require.NoError(b, err)
	}
}

func BenchmarkPay(b *testing.B) { benchmarkFullBlocks(testCases["pay"], b) }

func init() {
	testCases = make(map[string]testParams)

	// Disable deadlock checking library
	deadlock.Opts.Disable = true

	// Payments
	params := testParams{
		txType: "pay",
		name:   "pay",
	}
	testCases[params.name] = params
}
