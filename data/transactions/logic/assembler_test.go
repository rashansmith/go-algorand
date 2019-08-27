package logic

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

// Check that assembly output is stable across time.
func TestAssemble(t *testing.T) {
	// UPDATE PROCEDURE:
	// Run test. It should pass. If test is not passing, do not change this test, fix the assembler first.
	// Extend this test program text. It is preferrable to append instructions to the end so that the program byte hex is visually similar and also simply extended by some new bytes.
	// Copy hex string from failing test output into source.
	// Run test. It should pass.
	//
	// This doesn't have to be a sensible program to run, it just has to compile.
	text := `err
global Round
global MinTxnFee
global MinBalance
global MaxTxnLife
global TimeStamp
byte 0x1234
byte base64 aGVsbG8gd29ybGQh
byte base64(aGVsbG8gd29ybGQh)
byte b64 aGVsbG8gd29ybGQh
byte b64(aGVsbG8gd29ybGQh)
addr RWXCBB73XJITATVQFOI7MVUUQOL2PFDDSDUMW4H4T2SNSX4SEUOQ2MM7F4
txn Sender
txn Fee
txn FirstValid
txn LastValid
txn Note
txn Receiver
txn Amount
txn CloseRemainderTo
txn VotePK
txn SelectionPK
txn VoteFirst
txn VoteLast
arg 0
arg 1
//account Balance
sha256
keccak256
int 0x031337
int 0x1234567812345678
int 0x0034567812345678
int 0x0000567812345678
int 0x0000007812345678
+
// extra int pushes to satisfy typechecking on the ops that pop two ints
intc 0
-
intc 2
/
intc 1
*
intc 1
<
intc 1
>
intc 1
<=
intc 1
>=
intc 1
&&
intc 1
||
intc 1
==
intc 1
!=
intc 1
!
byte 0x4242
btoi
bytec 1
len
bytec 1
sha512_256
`
	program, err := AssembleString(text)
	require.NoError(t, err)
	// check that compilation is stable over time and we assemble to the same bytes this month that we did last month.
	expectedBytes, _ := hex.DecodeString("2005b7a60cf8acd19181cf959a12f8acd19181cf951af8acd19181cf15f8acd191810f26040212340c68656c6c6f20776f726c6421208dae2087fbba51304eb02b91f656948397a7946390e8cb70fc9ea4d95f92251d024242003200320132023203320428292929292a3100310131023103310431053106310731083109310a310b2d2e0102222324252104082209240a230b230c230d230e230f231023112312231323142b1729152903")
	if bytes.Compare(expectedBytes, program) != 0 {
		// this print is for convenience if the program has been changed. the hex string can be copy pasted back in as a new expected result.
		t.Log(hex.EncodeToString(program))
	}
	require.Equal(t, expectedBytes, program)
}

func TestOpUint(t *testing.T) {
	ops := OpStream{}
	err := ops.Uint(0xcafebabe)
	require.NoError(t, err)
	program, err := ops.Bytes()
	require.NoError(t, err)
	s := hex.EncodeToString(program)
	require.Equal(t, "2001bef5fad70c22", s)
}

func TestOpUint64(t *testing.T) {
	ops := OpStream{}
	err := ops.Uint(0xcafebabecafebabe)
	require.NoError(t, err)
	program, err := ops.Bytes()
	require.NoError(t, err)
	s := hex.EncodeToString(program)
	require.Equal(t, "2001bef5fad7ecd7aeffca0122", s)
}

func TestOpBytes(t *testing.T) {
	ops := OpStream{}
	err := ops.ByteLiteral([]byte("abcdef"))
	program, err := ops.Bytes()
	require.NoError(t, err)
	s := hex.EncodeToString(program)
	require.Equal(t, "26010661626364656628", s)
}

func TestAssembleInt(t *testing.T) {
	text := "int 0xcafebabe"
	program, err := AssembleString(text)
	require.NoError(t, err)
	s := hex.EncodeToString(program)
	require.Equal(t, "2001bef5fad70c22", s)
}

/*
test values generated in Python
python3
import base64
raw='abcdef'
base64.b64encode(raw.encode())
base64.b32encode(raw.encode())
base64.b16encode(raw.encode())
*/

func TestAssembleBytes(t *testing.T) {
	variations := []string{
		"byte b32 MFRGGZDFMY",
		"byte base32 MFRGGZDFMY",
		"byte base32(MFRGGZDFMY)",
		"byte b32(MFRGGZDFMY)",
		"byte b64 YWJjZGVm",
		"byte base64 YWJjZGVm",
		"byte b64(YWJjZGVm)",
		"byte base64(YWJjZGVm)",
		"byte 0x616263646566",
	}
	for _, vi := range variations {
		program, err := AssembleString(vi)
		require.NoError(t, err)
		s := hex.EncodeToString(program)
		require.Equal(t, "26010661626364656628", s)
	}
}
