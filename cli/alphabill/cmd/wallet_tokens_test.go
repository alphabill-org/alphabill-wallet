package cmd

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	tw "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	"github.com/holiman/uint256"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type accountManagerMock struct {
	keyHash       []byte
	recordedIndex uint64
}

func (a *accountManagerMock) GetAccountKey(accountIndex uint64) (*wallet.AccountKey, error) {
	a.recordedIndex = accountIndex
	return &wallet.AccountKey{PubKeyHash: &wallet.KeyHashes{Sha256: a.keyHash}}, nil
}

func TestParsePredicateClause(t *testing.T) {
	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	tests := []struct {
		clause    string
		predicate []byte
		index     uint64
		err       string
	}{
		{
			clause:    "",
			predicate: script.PredicateAlwaysTrue(),
		}, {
			clause: "foo",
			err:    "invalid predicate clause",
		},
		{
			clause:    "0x53510087",
			predicate: []byte{0x53, 0x51, 0x00, 0x87},
		},
		{
			clause:    "true",
			predicate: script.PredicateAlwaysTrue(),
		},
		{
			clause:    "false",
			predicate: script.PredicateAlwaysFalse(),
		},
		{
			clause: "ptpkh:",
			err:    "invalid predicate clause",
		},
		{
			clause:    "ptpkh",
			index:     uint64(0),
			predicate: script.PredicatePayToPublicKeyHashDefault(mock.keyHash),
		},
		{
			clause: "ptpkh:0",
			err:    "invalid key number: 0",
		},
		{
			clause:    "ptpkh:2",
			index:     uint64(1),
			predicate: script.PredicatePayToPublicKeyHashDefault(mock.keyHash),
		},
		{
			clause:    "ptpkh:0x0102",
			predicate: script.PredicatePayToPublicKeyHashDefault(mock.keyHash),
		},
		{
			clause: "ptpkh:0X",
			err:    "invalid predicate clause",
		},
	}

	for _, tt := range tests {
		t.Run(tt.clause, func(t *testing.T) {
			mock.recordedIndex = 0
			predicate, err := parsePredicateClause(tt.clause, mock)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.predicate, predicate)
			require.Equal(t, tt.index, mock.recordedIndex)
		})
	}
}

func TestParsePredicateArgument(t *testing.T) {
	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	tests := []struct {
		input string
		// expectations:
		result tokens.Predicate
		accKey uint64
		err    string
	}{
		{
			input:  "",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "empty",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "true",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "false",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "0x",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "0x5301",
			result: []byte{0x53, 0x01},
		},
		{
			input: "ptpkh:0",
			err:   "invalid key number: 0",
		},
		{
			input:  "ptpkh",
			accKey: uint64(1),
		},
		{
			input:  "ptpkh:1",
			accKey: uint64(1),
		},
		{
			input:  "ptpkh:10",
			accKey: uint64(10),
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			argument, err := parsePredicateArgument(tt.input, mock)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				if tt.accKey > 0 {
					require.Equal(t, tt.accKey, argument.AccountNumber)
				} else {
					require.Equal(t, tt.result, argument.Argument)
				}
			}
		})
	}
}

func TestDecodeHexOrEmpty(t *testing.T) {
	empty := []byte{}
	tests := []struct {
		input  string
		result []byte
		err    string
	}{
		{
			input:  "",
			result: empty,
		},
		{
			input:  "empty",
			result: empty,
		},
		{
			input:  "0x",
			result: empty,
		},
		{
			input: "0x534",
			err:   "odd length hex string",
		},
		{
			input: "0x53q",
			err:   "invalid byte",
		},
		{
			input:  "53",
			result: []byte{0x53},
		},
		{
			input:  "0x5354",
			result: []byte{0x53, 0x54},
		},
		{
			input:  "5354",
			result: []byte{0x53, 0x54},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			res, err := decodeHexOrEmpty(tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.result, res)
			}
		})
	}
}

func TestTokensWithRunningPartition(t *testing.T) {
	partition, unitState := startTokensPartition(t)
	startRPCServer(t, partition, listenAddr)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1 := createNewTokenWallet(t, "w1", dialAddr)
	w1.Shutdown()
	w2 := createNewTokenWallet(t, "w2", dialAddr)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	verifyStdout(t, execTokensCmd(t, "w1", ""), "Error: must specify a subcommand like new-type, send etc")
	verifyStdout(t, execTokensCmd(t, "w1", "new-type"), "Error: must specify a subcommand: fungible|non-fungible")

	testFungibleTokensWithRunningPartition(t, partition, unitState, w2key)

	testNFTsWithRunningPartition(t, partition, unitState, w2key)

	testTokenSubtypingWithRunningPartition(t, partition, unitState, w2key)
}

func testFungibleTokensWithRunningPartition(t *testing.T, partition *testpartition.AlphabillPartition, unitState tokens.TokenState, w2key *wallet.AccountKey) {
	typeID1 := randomID(t)
	// fungible token types
	symbol1 := "AB"
	execTokensCmdWithError(t, "w1", "new-type fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible --sync true --symbol %s -u %s --type %X", symbol1, dialAddr, typeID1))
	ensureUnit(t, unitState, uint256.NewInt(0).SetBytes(typeID1))
	// mint tokens
	crit := func(amount uint64) func(tx *txsystem.Transaction) bool {
		return func(tx *txsystem.Transaction) bool {
			if tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintFungibleTokenAttributes" {
				attrs := &tokens.MintFungibleTokenAttributes{}
				require.NoError(t, tx.TransactionAttributes.UnmarshalTo(attrs))
				return attrs.Value == amount
			}
			return false
		}
	}
	execTokensCmd(t, "w1", fmt.Sprintf("new fungible --sync false -u %s --type %X --amount 3", dialAddr, typeID1))
	execTokensCmd(t, "w1", fmt.Sprintf("new fungible --sync false -u %s --type %X --amount 5", dialAddr, typeID1))
	execTokensCmd(t, "w1", fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 9", dialAddr, typeID1))
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(3)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(5)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(9)), test.WaitDuration, test.WaitTick)
	// check w2 is empty
	verifyStdout(t, execTokensCmd(t, "w2", fmt.Sprintf("list fungible --sync true -u %s", dialAddr)), "No tokens")
	// transfer tokens w1 -> w2
	execTokensCmd(t, "w1", fmt.Sprintf("send fungible -u %s --type %X --amount 6 --address 0x%X -k 1", dialAddr, typeID1, w2key.PubKey)) //split (9=>6+3)
	execTokensCmd(t, "w1", fmt.Sprintf("send fungible -u %s --type %X --amount 6 --address 0x%X -k 1", dialAddr, typeID1, w2key.PubKey)) //transfer (5) + split (3=>2+1)
	out := execTokensCmd(t, "w2", fmt.Sprintf("list fungible -u %s", dialAddr))
	verifyStdout(t, out, "amount='6'", "amount='5'", "amount='1'", "Symbol='AB'")
	verifyStdoutNotExists(t, out, "Symbol=''", "token-type=''")
	//check what is left in w1
	verifyStdout(t, execTokensCmd(t, "w1", fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='3'", "amount='2'")

}

func testNFTsWithRunningPartition(t *testing.T, partition *testpartition.AlphabillPartition, unitState tokens.TokenState, w2key *wallet.AccountKey) {
	// non-fungible token types
	typeID := randomID(t)
	nftID := randomID(t)
	symbol := "ABNFT"
	execTokensCmdWithError(t, "w1", "new-type non-fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, "w1", fmt.Sprintf("new-type non-fungible --sync true --symbol %s -u %s --type %X", symbol, dialAddr, typeID))
	ensureUnitBytes(t, unitState, typeID)
	// mint NFT
	execTokensCmd(t, "w1", fmt.Sprintf("new non-fungible --sync true -u %s --type %X --token-identifier %X", dialAddr, typeID, nftID))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID)
	}), test.WaitDuration, test.WaitTick)
	// transfer NFT
	execTokensCmd(t, "w1", fmt.Sprintf("send non-fungible --sync false -u %s --token-identifier %X --address 0x%X -k 1", dialAddr, nftID, w2key.PubKey))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.TransferNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID)
	}), test.WaitDuration, test.WaitTick)
	verifyStdout(t, execTokensCmd(t, "w2", fmt.Sprintf("list non-fungible -u %s", dialAddr)), fmt.Sprintf("ID='%X'", nftID))
	//check what is left in w1, nothing, that is
	verifyStdout(t, execTokensCmd(t, "w1", fmt.Sprintf("list non-fungible -u %s", dialAddr)), "No tokens")
	// list token types
	verifyStdout(t, execTokensCmd(t, "w1", fmt.Sprintf("list-types")), "symbol=ABNFT, kind: 0x90", "symbol=AB, kind: 0x50")
	verifyStdout(t, execTokensCmd(t, "w1", fmt.Sprintf("list-types fungible")), "symbol=AB, kind: 0x50")
	verifyStdout(t, execTokensCmd(t, "w1", fmt.Sprintf("list-types non-fungible")), "symbol=ABNFT, kind: 0x90")
}

func testTokenSubtypingWithRunningPartition(t *testing.T, partition *testpartition.AlphabillPartition, unitState tokens.TokenState, w2key *wallet.AccountKey) {
	symbol1 := "AB"
	// test subtyping
	typeID11 := randomID(t)
	typeID12 := randomID(t)
	typeID13 := randomID(t)
	typeID14 := randomID(t)
	//push bool false, equal; to satisfy: 5100
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s", dialAddr, symbol1, typeID11, "0x53510087"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID11)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID11)
	//second type inheriting the first one and setting subtype clause to ptpkh
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s --parent-type %X --creation-input %s", dialAddr, symbol1, typeID12, "ptpkh", typeID11, "0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID12)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID12)
	//third type needs to satisfy both parents, immediate parent with ptpkh, grandparent with 0x535100
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s --parent-type %X --creation-input %s", dialAddr, symbol1, typeID13, "true", typeID12, "ptpkh,0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID13)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID13)
	//4th type
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s --parent-type %X --creation-input %s", dialAddr, symbol1, typeID14, "true", typeID13, "empty,ptpkh,0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID14)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID14)
}

func TestListTokensCommandInputs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		accountNumber int
		expectedKind  tw.TokenKind
	}{
		{
			name:          "list all tokens",
			args:          []string{},
			accountNumber: -1, // all tokens
			expectedKind:  tw.Any,
		},
		{
			name:          "list account tokens",
			args:          []string{"--key", "3"},
			accountNumber: 3,
			expectedKind:  tw.Any,
		},
		{
			name:          "list all fungible tokens",
			args:          []string{"fungible"},
			accountNumber: -1,
			expectedKind:  tw.FungibleToken,
		},
		{
			name:          "list account fungible tokens",
			args:          []string{"fungible", "--key", "4"},
			accountNumber: 4,
			expectedKind:  tw.FungibleToken,
		},
		{
			name:          "list all non-fungible tokens",
			args:          []string{"non-fungible"},
			accountNumber: -1,
			expectedKind:  tw.NonFungibleToken,
		},
		{
			name:          "list account non-fungible tokens",
			args:          []string{"non-fungible", "--key", "5"},
			accountNumber: 5,
			expectedKind:  tw.NonFungibleToken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdList(&walletConfig{}, func(cmd *cobra.Command, config *walletConfig, kind tw.TokenKind, accountNumber *int) error {
				require.Equal(t, tt.accountNumber, *accountNumber)
				require.Equal(t, tt.expectedKind, kind)
				exec = true
				return nil
			})
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			require.NoError(t, err)
			require.True(t, exec)
		})
	}
}

func ensureUnitBytes(t *testing.T, state tokens.TokenState, id []byte) {
	ensureUnit(t, state, uint256.NewInt(0).SetBytes(id))
}

func ensureUnit(t *testing.T, state tokens.TokenState, id *uint256.Int) {
	unit, err := state.GetUnit(id)
	require.NoError(t, err)
	require.NotNil(t, unit)
}

func startTokensPartition(t *testing.T) (*testpartition.AlphabillPartition, tokens.TokenState) {
	tokensState, err := rma.New(&rma.Config{
		HashAlgorithm: gocrypto.SHA256,
	})
	require.NoError(t, err)
	require.NotNil(t, tokensState)
	network, err := testpartition.NewNetwork(1,
		func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
			system, err := tokens.New(tokens.WithState(tokensState))
			require.NoError(t, err)
			return system
		}, tokens.DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = network.Close()
	})
	return network, tokensState
}

func createNewTokenWallet(t *testing.T, name string, addr string) *tw.Wallet {
	mw := createNewNamedWallet(t, name, addr)

	w, err := tw.Load(mw, false)
	require.NoError(t, err)
	require.NotNil(t, w)

	return w
}

func execTokensCmdWithError(t *testing.T, walletName string, command string, expectedError string) {
	_, err := doExecTokensCmd(walletName, command)
	require.ErrorContains(t, err, expectedError)
}

func execTokensCmd(t *testing.T, walletName string, command string) *testConsoleWriter {
	outputWriter, err := doExecTokensCmd(walletName, command)
	require.NoError(t, err)

	return outputWriter
}

func doExecTokensCmd(walletName string, command string) (*testConsoleWriter, error) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	homeDir := path.Join(os.TempDir(), walletName)

	cmd := New()
	args := "wallet token --log-level DEBUG --home " + homeDir + " " + command
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	return outputWriter, cmd.addAndExecuteCommand(context.Background())
}

func randomID(t *testing.T) tw.TokenID {
	id, err := tw.RandomID()
	require.NoError(t, err)
	return id
}

func TestListTokensTypesCommandInputs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedKind tw.TokenKind
	}{
		{
			name:         "list all tokens",
			args:         []string{},
			expectedKind: tw.Any,
		},
		{
			name:         "list all fungible tokens",
			args:         []string{"fungible"},
			expectedKind: tw.FungibleTokenType,
		},
		{
			name:         "list all non-fungible tokens",
			args:         []string{"non-fungible"},
			expectedKind: tw.NonFungibleTokenType,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdListTypes(&walletConfig{}, func(cmd *cobra.Command, config *walletConfig, kind tw.TokenKind) error {
				require.Equal(t, tt.expectedKind, kind)
				exec = true
				return nil
			})
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			require.NoError(t, err)
			require.True(t, exec)
		})
	}
}