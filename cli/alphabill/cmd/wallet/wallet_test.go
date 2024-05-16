package wallet

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/util"
	abrpc "github.com/alphabill-org/alphabill/rpc"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	moneywallet "github.com/alphabill-org/alphabill-wallet/wallet/money"
)

func TestWalletCreateCmd(t *testing.T) {
	outputWriter := &testutils.TestConsoleWriter{}
	homeDir := testutils.SetupTestHomeDir(t, "wallet-test")
	wcmd := NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter})
	wcmd.SetArgs([]string{"create"})
	err := wcmd.Execute()
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
	testutils.VerifyStdout(t, outputWriter,
		"The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")
}

func TestWalletCreateCmd_encrypt(t *testing.T) {
	outputWriter := &testutils.TestConsoleWriter{}
	homeDir := testutils.SetupTestHomeDir(t, "wallet-test")
	wcmd := NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter})
	pw := "123456"
	wcmd.SetArgs([]string{"create", "--pn", pw})
	err := wcmd.Execute()
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
	testutils.VerifyStdout(t, outputWriter,
		"The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")

	// verify wallet is encrypted
	// failing case: missing password
	wcmd = NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter})
	wcmd.SetArgs([]string{"add-key"})
	err = wcmd.Execute()
	require.ErrorContains(t, err, "invalid password")

	// failing case: wrong password
	wcmd = NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter})
	wcmd.SetArgs([]string{"add-key", "--pn", "123"})
	err = wcmd.Execute()
	require.ErrorContains(t, err, "invalid password")

	// passing case:
	wcmd = NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter})
	wcmd.SetArgs([]string{"add-key", "--pn", pw})
	err = wcmd.Execute()
	require.NoError(t, err)
}

func TestWalletCreateCmd_invalidSeed(t *testing.T) {
	outputWriter := &testutils.TestConsoleWriter{}
	homeDir := testutils.SetupTestHomeDir(t, "wallet-test")
	wcmd := NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter})
	wcmd.SetArgs([]string{"create", "-s", "--wallet-location", homeDir})
	err := wcmd.Execute()
	require.EqualError(t, err, `invalid value "--wallet-location" for flag "seed" (mnemonic)`)
	require.False(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
}

func TestWalletGetBalanceCmd(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
			UnitID:         money.NewBillID(nil, []byte{1}),
			Data:           money.BillData{V: 15 * 1e8},
			OwnerPredicate: testutils.TestPubKey0Hash(t),
		}),
	))

	walletCmd := newWalletCmdExecutor("--rpc-url", rpcUrl).WithHome(homedir)
	stdout := walletCmd.Exec(t, "get-balance")
	testutils.VerifyStdout(t, stdout, "#1 15", "Total 15")
}

func TestWalletGetBalanceKeyCmdKeyFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic(), testutils.WithNumberOfAccounts(2))
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
		UnitID:         money.NewBillID(nil, []byte{1}),
		Data:           money.BillData{V: 15 * 1e8},
		OwnerPredicate: testutils.TestPubKey1Hash(t),
	})))

	walletCmd := newWalletCmdExecutor("--rpc-url", rpcUrl).WithHome(homedir)
	stdout := walletCmd.Exec(t, "get-balance", "--key", "2")
	testutils.VerifyStdout(t, stdout, "#2 15")
	testutils.VerifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdTotalFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
		UnitID:         money.NewBillID(nil, []byte{1}),
		Data:           money.BillData{V: 15 * 1e8},
		OwnerPredicate: testutils.TestPubKey0Hash(t),
	})))

	walletCmd := newWalletCmdExecutor("--rpc-url", rpcUrl).WithHome(homedir)
	stdout := walletCmd.Exec(t, "get-balance", "--total")
	testutils.VerifyStdout(t, stdout, "Total 15")
	testutils.VerifyStdoutNotExists(t, stdout, "#1 15")
}

func TestWalletGetBalanceCmdTotalWithKeyFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
		UnitID:         money.NewBillID(nil, []byte{1}),
		Data:           money.BillData{V: 15 * 1e8},
		OwnerPredicate: testutils.TestPubKey0Hash(t),
	})))

	walletCmd := newWalletCmdExecutor("--rpc-url", rpcUrl).WithHome(homedir)
	stdout := walletCmd.Exec(t, "get-balance", "--key", "1", "--total")
	testutils.VerifyStdout(t, stdout, "#1 15")
	testutils.VerifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdQuietFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
		UnitID:         money.NewBillID(nil, []byte{1}),
		Data:           money.BillData{V: 15 * 1e8},
		OwnerPredicate: testutils.TestPubKey0Hash(t),
	})))

	walletCmd := newWalletCmdExecutor("--rpc-url", rpcUrl).WithHome(homedir)

	// verify quiet flag does nothing if no key or total flag is not provided
	stdout := walletCmd.Exec(t, "get-balance", "--quiet")
	testutils.VerifyStdout(t, stdout, "#1 15")
	testutils.VerifyStdout(t, stdout, "Total 15")

	// verify quiet with total
	stdout = walletCmd.Exec(t, "get-balance", "--quiet", "--total")
	testutils.VerifyStdout(t, stdout, "15")
	testutils.VerifyStdoutNotExists(t, stdout, "#1 15")

	// verify quiet with key
	stdout = walletCmd.Exec(t, "get-balance", "--quiet", "--key", "1")
	testutils.VerifyStdout(t, stdout, "15")
	testutils.VerifyStdoutNotExists(t, stdout, "Total 15")

	// verify quiet with key and total (total is not shown if key is provided)
	stdout = walletCmd.Exec(t, "get-balance", "--quiet", "--key", "1", "--total")
	testutils.VerifyStdout(t, stdout, "15")
	testutils.VerifyStdoutNotExists(t, stdout, "#1 15")
}

func TestPubKeysCmd(t *testing.T) {
	am, homedir := testutils.CreateNewWallet(t)
	pk, err := am.GetPublicKey(0)
	require.NoError(t, err)
	am.Close()

	walletCmd := newWalletCmdExecutor().WithHome(homedir)
	stdout := walletCmd.Exec(t, "get-pubkeys")
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 "+hexutil.Encode(pk))
}

func TestSendingFailsWithInsufficientBalance(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
			UnitID:         money.NewBillID(nil, []byte{8}),
			Data:           money.BillData{V: 5 * 1e8},
			OwnerPredicate: testutils.TestPubKey0Hash(t),
		}),
		mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
			UnitID:         money.NewFeeCreditRecordID(nil, testutils.TestPubKey0Hash(t)),
			Data:           fc.FeeCreditRecord{Balance: 1e8},
			OwnerPredicate: testutils.TestPubKey0Hash(t),
		}),
	))

	walletCmd := newWalletCmdExecutor("--rpc-url", rpcUrl).WithHome(homedir)
	walletCmd.ExecWithError(t, "insufficient balance for transaction",
		"send", "--amount", "10", "--address", "0x" + testutils.TestPubKey1Hex)
}

func Test_groupPubKeysAndAmounts(t *testing.T) {
	t.Run("count of keys and amounts do not match", func(t *testing.T) {
		data, err := groupPubKeysAndAmounts(nil, []string{"1"})
		require.EqualError(t, err, `must specify the same amount of addresses and amounts (got 0 vs 1)`)
		require.Empty(t, data)

		data, err = groupPubKeysAndAmounts([]string{}, []string{"1"})
		require.EqualError(t, err, `must specify the same amount of addresses and amounts (got 0 vs 1)`)
		require.Empty(t, data)

		data, err = groupPubKeysAndAmounts([]string{"key"}, []string{"1", "2"})
		require.EqualError(t, err, `must specify the same amount of addresses and amounts (got 1 vs 2)`)
		require.Empty(t, data)

		data, err = groupPubKeysAndAmounts([]string{"key"}, nil)
		require.EqualError(t, err, `must specify the same amount of addresses and amounts (got 1 vs 0)`)
		require.Empty(t, data)

		data, err = groupPubKeysAndAmounts([]string{"key"}, []string{})
		require.EqualError(t, err, `must specify the same amount of addresses and amounts (got 1 vs 0)`)
		require.Empty(t, data)

		data, err = groupPubKeysAndAmounts([]string{"key1", "key2"}, []string{"1"})
		require.EqualError(t, err, `must specify the same amount of addresses and amounts (got 2 vs 1)`)
		require.Empty(t, data)
	})

	t.Run("single address and amount", func(t *testing.T) {
		data, err := groupPubKeysAndAmounts([]string{"0x01"}, []string{"1"})
		require.NoError(t, err)
		require.Equal(t, []moneywallet.ReceiverData{{PubKey: []byte{1}, Amount: 100000000}}, data)
	})

	t.Run("address and amount pair", func(t *testing.T) {
		data, err := groupPubKeysAndAmounts([]string{"0x01", "0x02"}, []string{"1", "2"})
		require.NoError(t, err)
		require.Equal(t, []moneywallet.ReceiverData{
			{PubKey: []byte{1}, Amount: 100000000},
			{PubKey: []byte{2}, Amount: 200000000},
		}, data)
	})
}

func Test_parseRefNumbers(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		ref, err := parseReferenceNumber("")
		require.NoError(t, err)
		require.Empty(t, ref)
	})

	t.Run("string too long", func(t *testing.T) {
		ref, err := parseReferenceNumber(strings.Repeat("A", 33))
		require.EqualError(t, err, `maximum allowed length of the reference number is 32 bytes, argument is 33 bytes`)
		require.Empty(t, ref)

		// utf-8, single character might require multiple bytes!
		ref, err = parseReferenceNumber(strings.Repeat("A", 31) + "Ö")
		require.EqualError(t, err, `maximum allowed length of the reference number is 32 bytes, argument is 33 bytes`)
		require.Empty(t, ref)
	})

	t.Run("bytes too long", func(t *testing.T) {
		ref, err := parseReferenceNumber("0x" + strings.Repeat("0", 66))
		require.EqualError(t, err, `maximum allowed length of the reference number is 32 bytes, argument is 33 bytes`)
		require.Empty(t, ref)
	})

	t.Run("invalid hex encoding", func(t *testing.T) {
		ref, err := parseReferenceNumber("0xInvalid")
		require.EqualError(t, err, `decoding reference number from hex string to binary: encoding/hex: invalid byte: U+0049 'I'`)
		require.Empty(t, ref)

		ref, err = parseReferenceNumber("0x123")
		require.EqualError(t, err, `decoding reference number from hex string to binary: encoding/hex: odd length hex string`)
		require.Empty(t, ref)
	})

	t.Run("valid inputs", func(t *testing.T) {
		ref, err := parseReferenceNumber("Ref Number")
		require.NoError(t, err)
		require.Equal(t, []byte("Ref Number"), ref)

		ref, err = parseReferenceNumber("0xAABBCC")
		require.NoError(t, err)
		require.Equal(t, []byte{0xAA, 0xBB, 0xCC}, ref)
	})
}
