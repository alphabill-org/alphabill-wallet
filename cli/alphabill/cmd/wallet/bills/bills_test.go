package bills

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/predicates/templates"
	abrpc "github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

func TestWalletBillsListCmd_EmptyWallet(t *testing.T) {
	rpcUrl := mocksrv.StartServer(t, mocksrv.NewRpcServerMock())
	homedir := testutils.CreateNewTestWallet(t)

	stdout, err := execBillsCommand(t, homedir, "list --rpc-url "+rpcUrl)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Account #1 - empty")
}

func TestWalletBillsListCmd_Single(t *testing.T) {
	rpcUrl := mocksrv.StartServer(t, mocksrv.NewRpcServerMock(mocksrv.WithOwnerBill(&abrpc.Unit[any]{
		UnitID:         money.NewBillID(nil, []byte{1}),
		Data:           money.BillData{V: 1e8},
		OwnerPredicate: testutils.TestPubKey0Hash(t),
	})))
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())

	stdout, err := execBillsCommand(t, homedir, "list --rpc-url "+rpcUrl)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00")
}

func TestWalletBillsListCmd_Multiple(t *testing.T) {
	addr := mocksrv.StartServer(t, mocksrv.NewRpcServerMock(
		mocksrv.WithOwnerBill(&abrpc.Unit[any]{UnitID: money.NewBillID(nil, []byte{1}), Data: money.BillData{V: 1}, OwnerPredicate: testutils.TestPubKey0Hash(t)}),
		mocksrv.WithOwnerBill(&abrpc.Unit[any]{UnitID: money.NewBillID(nil, []byte{2}), Data: money.BillData{V: 2}, OwnerPredicate: testutils.TestPubKey0Hash(t)}),
		mocksrv.WithOwnerBill(&abrpc.Unit[any]{UnitID: money.NewBillID(nil, []byte{3}), Data: money.BillData{V: 3}, OwnerPredicate: testutils.TestPubKey0Hash(t)}),
		mocksrv.WithOwnerBill(&abrpc.Unit[any]{UnitID: money.NewBillID(nil, []byte{4}), Data: money.BillData{V: 4}, OwnerPredicate: testutils.TestPubKey0Hash(t)}),
	))
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())

	stdout, err := execBillsCommand(t, homedir, "list --rpc-url "+addr)
	require.NoError(t, err)
	require.Len(t, stdout.Lines, 5)
	require.Equal(t, stdout.Lines[0], "Account #1")
	require.Equal(t, stdout.Lines[1], "#1 0x000000000000000000000000000000000000000000000000000000000000000100 0.000'000'01")
	require.Equal(t, stdout.Lines[2], "#2 0x000000000000000000000000000000000000000000000000000000000000000200 0.000'000'02")
	require.Equal(t, stdout.Lines[3], "#3 0x000000000000000000000000000000000000000000000000000000000000000300 0.000'000'03")
	require.Equal(t, stdout.Lines[4], "#4 0x000000000000000000000000000000000000000000000000000000000000000400 0.000'000'04")
}

func TestWalletBillsListCmd_ExtraAccount(t *testing.T) {
	rpcUrl := mocksrv.StartServer(t, mocksrv.NewRpcServerMock(
		mocksrv.WithOwnerBill(&abrpc.Unit[any]{UnitID: money.NewBillID(nil, []byte{1}), Data: money.BillData{V: 1}, OwnerPredicate: testutils.TestPubKey1Hash(t)}),
	))
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic(), testutils.WithNumberOfAccounts(2))

	// verify list bills for specific account only shows given account bills
	stdout, err := execBillsCommand(t, homedir, "list -k 2 --rpc-url "+rpcUrl)
	require.NoError(t, err)
	lines := stdout.Lines
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "Account #2")
	require.Contains(t, lines[1], "#1")
}

func TestWalletBillsListCmd_ExtraAccountTotal(t *testing.T) {
	rpcUrl := mocksrv.StartServer(t, mocksrv.NewRpcServerMock(
		mocksrv.WithOwnerBill(&abrpc.Unit[any]{UnitID: money.NewBillID(nil, []byte{1}), Data: money.BillData{V: 1e9}, OwnerPredicate: testutils.TestPubKey0Hash(t)}),
	))
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic(), testutils.WithNumberOfAccounts(2))

	// verify both accounts are listed
	stdout, err := execBillsCommand(t, homedir, "list --rpc-url "+rpcUrl)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Account #1")
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 10")
	testutils.VerifyStdout(t, stdout, "Account #2 - empty")
}

func TestWalletBillsListCmd_ShowUnswappedFlag(t *testing.T) {
	t.Skipf("implement --show-unswapped after dust collector refactor")
	//homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic(), testutils.WithNumberOfAccounts(2))
	//
	//// verify no -s flag sends includeDcBills=false by default
	//mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{
	//	CustomFullPath: "/" + client.ListBillsPath + "?includeDcBills=false&limit=100&pubkey=" + pubKey,
	//	CustomResponse: `{"bills": [{"value":"22222222"}]}`})
	//
	//stdout, err := execBillsCommand(t, homedir, "list --rpc-url "+addr.Host)
	//require.NoError(t, err)
	//testutils.VerifyStdout(t, stdout, "#1 0x 0.222'222'22")
	//mockServer.Close()
	//
	//// verify -s flag sends includeDcBills=true
	//mockServer, addr = mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{
	//	CustomFullPath: "/" + client.ListBillsPath + "?includeDcBills=true&limit=100&pubkey=" + pubKey,
	//	CustomResponse: `{"bills": [{"value":"33333333"}]}`})
	//
	//stdout, err = execBillsCommand(t, homedir, "list --rpc-url "+addr.Host+" -s")
	//require.NoError(t, err)
	//testutils.VerifyStdout(t, stdout, "#1 0x 0.333'333'33")
	//mockServer.Close()
}

func TestWalletBillsListCmd_ShowLockedBills(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartServer(t, mocksrv.NewRpcServerMock(
		mocksrv.WithOwnerBill(&abrpc.Unit[any]{UnitID: money.NewBillID(nil, []byte{1}), Data: money.BillData{V: 1e8, Locked: wallet.LockReasonAddFees}, OwnerPredicate: testutils.TestPubKey0Hash(t)}),
		mocksrv.WithOwnerBill(&abrpc.Unit[any]{UnitID: money.NewBillID(nil, []byte{2}), Data: money.BillData{V: 1e8, Locked: wallet.LockReasonReclaimFees}, OwnerPredicate: testutils.TestPubKey0Hash(t)}),
		mocksrv.WithOwnerBill(&abrpc.Unit[any]{UnitID: money.NewBillID(nil, []byte{3}), Data: money.BillData{V: 1e8, Locked: wallet.LockReasonCollectDust}, OwnerPredicate: testutils.TestPubKey0Hash(t)}),
	))

	stdout, err := execBillsCommand(t, homedir, "list --rpc-url "+rpcUrl)
	require.NoError(t, err)
	require.Len(t, stdout.Lines, 4)
	require.Equal(t, stdout.Lines[1], "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00 (locked for adding fees)")
	require.Equal(t, stdout.Lines[2], "#2 0x000000000000000000000000000000000000000000000000000000000000000200 1.000'000'00 (locked for reclaiming fees)")
	require.Equal(t, stdout.Lines[3], "#3 0x000000000000000000000000000000000000000000000000000000000000000300 1.000'000'00 (locked for dust collection)")
}

func TestWalletBillsLockUnlockCmd_Ok(t *testing.T) {
	// setup network
	homedir, accountKey, rpcUrl, abNet := setupNetwork(t, nil)

	// add fee credit
	testutils.AddFeeCredit(t, 1e8, money.DefaultSystemIdentifier, accountKey, testutils.DefaultInitialBillID, nil, money.NewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256), nil, abNet.NodePartitions[money.DefaultSystemIdentifier])

	// lock bill
	stdout, err := execBillsCommand(t, homedir, fmt.Sprintf("lock --rpc-url %s --bill-id %s", rpcUrl, money.NewBillID(nil, []byte{1})))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Bill locked successfully.")

	// verify bill locked
	stdout, err = execBillsCommand(t, homedir, fmt.Sprintf("list --rpc-url %s", rpcUrl))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 99.000'000'00 (manually locked by user)")

	// unlock bill
	stdout, err = execBillsCommand(t, homedir, fmt.Sprintf("unlock --rpc-url %s --bill-id %s", rpcUrl, money.NewBillID(nil, []byte{1})))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Bill unlocked successfully.")

	// verify bill unlocked
	stdout, err = execBillsCommand(t, homedir, fmt.Sprintf("list --rpc-url %s", rpcUrl))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 99.000'000'00")
}

func TestWalletBillsLockUnlockCmd_Nok(t *testing.T) {
	rpcUrl := mocksrv.StartServer(t, mocksrv.NewRpcServerMock())
	homedir := testutils.CreateNewTestWallet(t)

	// lock bill
	_, err := execBillsCommand(t, homedir, fmt.Sprintf("lock --rpc-url %s --bill-id %s", rpcUrl, testutils.DefaultInitialBillID))
	require.ErrorContains(t, err, "not enough fee credit in wallet")

	// unlock bill
	_, err = execBillsCommand(t, homedir, fmt.Sprintf("unlock --rpc-url %s --bill-id %s", rpcUrl, testutils.DefaultInitialBillID))
	require.ErrorContains(t, err, "not enough fee credit in wallet")
}

func execBillsCommand(t *testing.T, homeDir, command string) (*testutils.TestConsoleWriter, error) {
	outputWriter := &testutils.TestConsoleWriter{}
	baseConfig := &types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml", Observe: testobserve.Default(t)}
	bcmd := NewBillsCmd(&types.WalletConfig{Base: baseConfig, WalletHomeDir: filepath.Join(homeDir, "wallet")})
	bcmd.SetArgs(strings.Split(command, " "))
	return outputWriter, bcmd.Execute()
}

// setupNetwork starts alphabill network.
// Starts money partition, and optionally any other partitions, with rpc servers up and running.
// The initial bill is set to the created wallet.
// Returns wallet homedir, money node url and reference to the network object.
func setupNetwork(t *testing.T, otherPartitions []*testpartition.NodePartition) (string, *account.AccountKey, string, *testpartition.AlphabillNetwork) {
	// create wallet
	am, homedir := testutils.CreateNewWallet(t)
	defer am.Close()
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	genesisConfig := &testutil.MoneyGenesisConfig{
		InitialBillID:      testutils.DefaultInitialBillID,
		InitialBillValue:   100 * 1e8,
		InitialBillOwner:   templates.NewP2pkh256BytesFromKey(accountKey.PubKey),
		DCMoneySupplyValue: 10000,
	}
	rpcUrl, abNet := testutils.SetupNetwork(t, genesisConfig, otherPartitions)
	return homedir, accountKey, rpcUrl, abNet
}
