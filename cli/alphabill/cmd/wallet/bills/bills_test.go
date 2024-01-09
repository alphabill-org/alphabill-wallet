package bills

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils/mocksrv"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

func TestWalletBillsListCmd_EmptyWallet(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{CustomBillList: `{"bills": []}`})
	defer mockServer.Close()
	stdout, err := execBillsCommand(t, homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Account #1 - empty")
}

func TestWalletBillsListCmd_Single(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{
		TargetBill: &wallet.Bill{
			Id:    money.NewBillID(nil, []byte{1}),
			Value: 1e8,
		}})
	defer mockServer.Close()

	// verify bill in list command
	stdout, err := execBillsCommand(t, homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00")
}

func TestWalletBillsListCmd_Multiple(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	billsList := ""
	for i := 1; i <= 4; i++ {
		billsList = billsList + fmt.Sprintf(`{"id":"%s","value":"%d","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},`, base64.StdEncoding.EncodeToString(money.NewBillID(nil, []byte{byte(i)})), i)
	}
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{CustomBillList: fmt.Sprintf(`{"bills": [%s]}`, strings.TrimSuffix(billsList, ","))})
	defer mockServer.Close()

	// verify list bills shows all 4 bills
	stdout, err := execBillsCommand(t, homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Account #1")
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 0.000'000'01")
	testutils.VerifyStdout(t, stdout, "#2 0x000000000000000000000000000000000000000000000000000000000000000200 0.000'000'02")
	testutils.VerifyStdout(t, stdout, "#3 0x000000000000000000000000000000000000000000000000000000000000000300 0.000'000'03")
	testutils.VerifyStdout(t, stdout, "#4 0x000000000000000000000000000000000000000000000000000000000000000400 0.000'000'04")
	require.Len(t, stdout.Lines, 5)
}

func TestWalletBillsListCmd_ExtraAccount(t *testing.T) {
	am, homedir := testutils.CreateNewWallet(t)
	_, _, err := am.AddAccount()
	require.NoError(t, err)
	am.Close()
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{
		TargetBill: &wallet.Bill{
			Id:    money.NewBillID(nil, []byte{1}),
			Value: 1,
		}})
	defer mockServer.Close()

	// verify list bills for specific account only shows given account bills
	stdout, err := execBillsCommand(t, homedir, "list -k 2 --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	lines := stdout.Lines
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "Account #2")
	require.Contains(t, lines[1], "#1")
}

func TestWalletBillsListCmd_ExtraAccountTotal(t *testing.T) {
	am, homedir := testutils.CreateNewWallet(t)
	_, pk, err := am.AddAccount()
	require.NoError(t, err)
	pubKey2 := hexutil.Encode(pk)
	am.Close()

	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{
		TargetBill: &wallet.Bill{
			Id:    money.NewBillID(nil, []byte{1}),
			Value: 1e9,
		},
		CustomFullPath: "/" + client.ListBillsPath + "?includeDcBills=false&limit=100&pubkey=" + pubKey2,
		CustomResponse: `{"bills": []}`})
	defer mockServer.Close()

	// verify both accounts are listed
	stdout, err := execBillsCommand(t, homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Account #1")
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 10")
	testutils.VerifyStdout(t, stdout, "Account #2 - empty")
}

func TestWalletBillsListCmd_ShowUnswappedFlag(t *testing.T) {
	am, homedir := testutils.CreateNewWallet(t)
	ac, err := am.GetAccountKey(0)
	require.NoError(t, err)
	pubKey := hexutil.Encode(ac.PubKey)
	am.Close()

	// verify no -s flag sends includeDcBills=false by default
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{
		CustomFullPath: "/" + client.ListBillsPath + "?includeDcBills=false&limit=100&pubkey=" + pubKey,
		CustomResponse: `{"bills": [{"value":"22222222"}]}`})

	stdout, err := execBillsCommand(t, homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 0x 0.222'222'22")
	mockServer.Close()

	// verify -s flag sends includeDcBills=true
	mockServer, addr = mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{
		CustomFullPath: "/" + client.ListBillsPath + "?includeDcBills=true&limit=100&pubkey=" + pubKey,
		CustomResponse: `{"bills": [{"value":"33333333"}]}`})

	stdout, err = execBillsCommand(t, homedir, "list --alphabill-api-uri "+addr.Host+" -s")
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 0x 0.333'333'33")
	mockServer.Close()
}

func TestWalletBillsListCmd_ShowLockedBills(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	var billsList []string
	for i := 1; i <= 3; i++ {
		idBase64 := base64.StdEncoding.EncodeToString(money.NewBillID(nil, []byte{byte(i)}))
		billsList = append(billsList, fmt.Sprintf(`{"id":"%s","locked":"%d","value":"100000000"}`, idBase64, i))
	}
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{CustomBillList: fmt.Sprintf(`{"bills": [%s]}`, strings.Join(billsList, ","))})
	defer mockServer.Close()
	stdout, err := execBillsCommand(t, homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00 (locked for adding fees)")
	testutils.VerifyStdout(t, stdout, "#2 0x000000000000000000000000000000000000000000000000000000000000000200 1.000'000'00 (locked for reclaiming fees)")
	testutils.VerifyStdout(t, stdout, "#3 0x000000000000000000000000000000000000000000000000000000000000000300 1.000'000'00 (locked for dust collection)")
}

func TestWalletBillsLockUnlockCmd_Ok(t *testing.T) {
	// create wallet
	am, homedir := testutils.CreateNewWallet(t)
	pubkey, err := am.GetPublicKey(0)
	require.NoError(t, err)
	ac, err := am.GetAccountKey(0)
	require.NoError(t, err)
	am.Close()

	// start money partition
	genesisConfig := &testutil.MoneyGenesisConfig{
		InitialBillID:      testutils.DefaultInitialBillID,
		InitialBillValue:   100 * 1e8,
		InitialBillOwner:   templates.NewP2pkh256BytesFromKey(pubkey),
		DCMoneySupplyValue: 10000,
	}
	moneyPartition := testutils.CreateMoneyPartition(t, genesisConfig, 1)

	_ = testutils.StartAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	testutils.StartPartitionRPCServers(t, moneyPartition)

	// start wallet backend
	addr, moneyBackendClient := testutils.StartMoneyBackend(t, moneyPartition, genesisConfig)

	// create fee credit
	fcrID := money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256)
	testutils.AddFeeCredit(t, 1e8, money.DefaultSystemIdentifier, ac, genesisConfig.InitialBillID, nil, fcrID, nil, moneyPartition)

	// wait for backend to index the fee transactions
	require.Eventually(t, func() bool {
		fcb, err := moneyBackendClient.GetFeeCreditBill(context.Background(), fcrID)
		require.NoError(t, err)
		return fcb.GetValue() > 0
	}, test.WaitDuration, test.WaitTick)

	// lock bill
	stdout, err := execBillsCommand(t, homedir, fmt.Sprintf("lock --alphabill-api-uri %s --bill-id %s", addr, testutils.DefaultInitialBillID))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Bill locked successfully.")

	// verify bill locked
	stdout, err = execBillsCommand(t, homedir, fmt.Sprintf("list --alphabill-api-uri %s", addr))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 99.000'000'00 (manually locked by user)")

	// unlock bill
	stdout, err = execBillsCommand(t, homedir, fmt.Sprintf("unlock --alphabill-api-uri %s --bill-id %s", addr, testutils.DefaultInitialBillID))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Bill unlocked successfully.")

	// verify bill unlocked
	stdout, err = execBillsCommand(t, homedir, fmt.Sprintf("list --alphabill-api-uri %s", addr))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 99.000'000'00")
}

func TestWalletBillsLockUnlockCmd_Nok(t *testing.T) {
	// TODO convert to unit test, no need to start entire network
	// create wallet
	am, homedir := testutils.CreateNewWallet(t)
	pubkey, err := am.GetPublicKey(0)
	require.NoError(t, err)
	am.Close()

	// start money partition
	genesisConfig := &testutil.MoneyGenesisConfig{
		InitialBillID:      testutils.DefaultInitialBillID,
		InitialBillValue:   2e8,
		InitialBillOwner:   templates.NewP2pkh256BytesFromKey(pubkey),
		DCMoneySupplyValue: 10000,
	}
	moneyPartition := testutils.CreateMoneyPartition(t, genesisConfig, 1)
	_ = testutils.StartAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	testutils.StartPartitionRPCServers(t, moneyPartition)
	testutils.StartPartitionRPCServers(t, moneyPartition)

	// start wallet backend
	addr, _ := testutils.StartMoneyBackend(t, moneyPartition, genesisConfig)

	// lock bill
	_, err = execBillsCommand(t, homedir, fmt.Sprintf("lock --alphabill-api-uri %s --bill-id %s", addr, testutils.DefaultInitialBillID))
	require.ErrorContains(t, err, "not enough fee credit in wallet")

	// unlock bill
	_, err = execBillsCommand(t, homedir, fmt.Sprintf("unlock --alphabill-api-uri %s --bill-id %s", addr, testutils.DefaultInitialBillID))
	require.ErrorContains(t, err, "not enough fee credit in wallet")
}

func execBillsCommand(t *testing.T, homeDir, command string) (*testutils.TestConsoleWriter, error) {
	outputWriter := &testutils.TestConsoleWriter{}
	baseConfig := &types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml", Observe: testobserve.Default(t)}
	bcmd := NewBillsCmd(&types.WalletConfig{Base: baseConfig, WalletHomeDir: filepath.Join(homeDir, "wallet")})
	bcmd.SetArgs(strings.Split(command, " "))
	return outputWriter, bcmd.Execute()
}
