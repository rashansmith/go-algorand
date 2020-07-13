#!/usr/bin/env bash

# first, you'll need to create an asset
${GOPATH}/bin/goal asset create -d ~/net1/Node --creator MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U  --total 100000 --unitname coin
# > Issued transaction from account MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U , txid JH7M5L43YLQ5DTRIVVBUUB2E4BFE7TPVAPPEGCUVNYSFRLT55Z3Q (fee 1000)
# > Transaction JH7M5L43YLQ5DTRIVVBUUB2E4BFE7TPVAPPEGCUVNYSFRLT55Z3Q still pending as of round 148369
# > Transaction JH7M5L43YLQ5DTRIVVBUUB2E4BFE7TPVAPPEGCUVNYSFRLT55Z3Q committed in round 148371
${GOPATH}/bin/goal asset info --creator MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U  -d ~/net1/Node --asset coin
# > Asset ID:         1
# > Creator:          MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U 
# > Asset name:       
# > Unit name:        e.g.Coin
# > Maximum issue:    100000 e.g.Coin
# > Reserve amount:   100000 e.g.Coin
# > Issued:           0 e.g.Coin
# > Default frozen:   false
# > Manager address:  MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U 
# > Reserve address:  MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U 
# > Freeze address:   MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U 
# > Clawback address: MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U 

# allow an account (we'll call her Alice) to accept this asset by sending a 0-asset transaction to yourself
${GOPATH}/bin/goal asset send --creator MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U  --assetid 1 --from UKV3I5X4W5XKSMBBCI63QXEAFSYGCYJ7J3VE622EDBIOVI5FKX4XIYP6HI --to UKV3I5X4W5XKSMBBCI63QXEAFSYGCYJ7J3VE622EDBIOVI5FKX4XIYP6HI --amount 0 -d ~/net1/Primary
# > Transaction ELLYMXT56IIZ57XT5U65QLERU5VQUDSU36AXI5IP4MPKQJDORKBQ still pending as of round 152630
# > Transaction ELLYMXT56IIZ57XT5U65QLERU5VQUDSU36AXI5IP4MPKQJDORKBQ committed in round 152632

# produce TEAL assembly for a limit order escrow: Alice will trade _more than_ 1000 Algos for at least 3/2 * 1000 of some asset
algotmpl -d `git rev-parse --show-toplevel`/tools/teal/templates limit-order-a --swapn 3 --swapd 2 --mintrd 1000 --own UKV3I5X4W5XKSMBBCI63QXEAFSYGCYJ7J3VE622EDBIOVI5FKX4XIYP6HI --fee 100000 --timeout 150000 --asset 1 > limit.teal

# compile TEAL assembly to TEAL bytecode
${GOPATH}/bin/goal clerk compile limit.teal 
# > limit.teal: MBCMDK3ILH2QWU24HDHP6GGJUP4EEWXLFKVYXLESALSGYNVJJUIYP7I3I4

# initialize the escrow by sending 1000000 microAlgos into it
${GOPATH}/bin/goal clerk send --from UKV3I5X4W5XKSMBBCI63QXEAFSYGCYJ7J3VE622EDBIOVI5FKX4XIYP6HI --to MBCMDK3ILH2QWU24HDHP6GGJUP4EEWXLFKVYXLESALSGYNVJJUIYP7I3I4   --amount 1000000 -d ~/net1/Primary
# > Sent 1000000 MicroAlgos from account UKV3I5X4W5XKSMBBCI63QXEAFSYGCYJ7J3VE622EDBIOVI5FKX4XIYP6HI to address MBCMDK3ILH2QWU24HDHP6GGJUP4EEWXLFKVYXLESALSGYNVJJUIYP7I3I4, transaction ID: Q564JY6YWGROG7QK6CCFFYIH4JT3OJ7S6GCBQDW3RMRG3JQ6HWMQ. Fee set to 1000
# > Transaction Q564JY6YWGROG7QK6CCFFYIH4JT3OJ7S6GCBQDW3RMRG3JQ6HWMQ still pending as of round 151473
# > Transaction Q564JY6YWGROG7QK6CCFFYIH4JT3OJ7S6GCBQDW3RMRG3JQ6HWMQ committed in round 151475

# at this point, Alice can publish limit.teal, and anyone can fill the order without interaction from her

# build the group transaction
# the first payment sends money (Algos) from Alice's escrow to the recipient (we'll call him Bob), closing the rest of the account to Alice
# the second payment sends money (the asset) from the Bob to the Alice
${GOPATH}/bin/goal clerk send --from-program limit.teal --to MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U --amount 2000 -d ~/net1/Primary -o test.tx
${GOPATH}/bin/goal asset send --creator MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U  --assetid 1 --from MBB7H6L36HNHHUSIQSBSOEDAZLTP7EMCTJFGXV63GF3UYYEYNYXPHR4I4U  --to UKV3I5X4W5XKSMBBCI63QXEAFSYGCYJ7J3VE622EDBIOVI5FKX4XIYP6HI --amount 20000 -d ~/net1/Node -o test2.tx
cat test.tx test2.tx > testcmb.tx
${GOPATH}/bin/goal clerk group -i testcmb.tx -o testgrp.tx

# Bob must sign his half of the transaction (Alice's half is authorized by the logic program's escrow)
# we must resplit the transaction (but this time they have the group fields set correctly)
${GOPATH}/bin/goal clerk split -i testgrp.tx -o testraw.tx
# > Wrote transaction 0 to testraw-0.tx
# > Wrote transaction 1 to testraw-1.tx
${GOPATH}/bin/goal clerk sign -i testraw-1.tx -o testraw-1.stx -d ~/net1/Node
cat testraw-0.tx testraw-1.stx > testraw.stx
${GOPATH}/bin/goal clerk inspect testraw.stx

# regroup the transactions and send the combined signed transactions to the network
${GOPATH}/bin/goal clerk rawsend -f testraw.stx -d ~/net1/Node
# > Raw transaction ID AJVGWKZJHN4HYOMJ45AW5RXVIBNYK3CFDUI737VZ2KQ3N7DVVQZQ issued
# > Raw transaction ID 5ALEOOLZYNYIMSQFILJ3OXS5B3JDBVEPB7DB4DKAPANBIC56TTUA issued
# > Transaction AJVGWKZJHN4HYOMJ45AW5RXVIBNYK3CFDUI737VZ2KQ3N7DVVQZQ still pending as of round 153304
# > Transaction AJVGWKZJHN4HYOMJ45AW5RXVIBNYK3CFDUI737VZ2KQ3N7DVVQZQ committed in round 153306
# > Transaction 5ALEOOLZYNYIMSQFILJ3OXS5B3JDBVEPB7DB4DKAPANBIC56TTUA committed in round 153306
