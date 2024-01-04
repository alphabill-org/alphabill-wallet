package wallet

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/backend/client"
)

func TestWalletBillsListCmd_EmptyWallet(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: `{"bills": []}`})
	defer mockServer.Close()
	stdout, err := execBillsCommand(testobserve.NewFactory(t), homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1 - empty")
}

func TestWalletBillsListCmd_Single(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{
		targetBill: &wallet.Bill{
			Id:    money.NewBillID(nil, []byte{1}),
			Value: 1e8,
		}})
	defer mockServer.Close()

	// verify bill in list command
	stdout, err := execBillsCommand(testobserve.NewFactory(t), homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00")
}

func TestWalletBillsListCmd_Multiple(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	billsList := ""
	for i := 1; i <= 4; i++ {
		billsList = billsList + fmt.Sprintf(`{"id":"%s","value":"%d","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},`, toBase64(money.NewBillID(nil, []byte{byte(i)})), i)
	}
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: fmt.Sprintf(`{"bills": [%s]}`, strings.TrimSuffix(billsList, ","))})
	defer mockServer.Close()

	// verify list bills shows all 4 bills
	stdout, err := execBillsCommand(testobserve.NewFactory(t), homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1")
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 0.000'000'01")
	verifyStdout(t, stdout, "#2 0x000000000000000000000000000000000000000000000000000000000000000200 0.000'000'02")
	verifyStdout(t, stdout, "#3 0x000000000000000000000000000000000000000000000000000000000000000300 0.000'000'03")
	verifyStdout(t, stdout, "#4 0x000000000000000000000000000000000000000000000000000000000000000400 0.000'000'04")
	require.Len(t, stdout.Lines, 5)
}

func TestWalletBillsListCmd_ExtraAccount(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	logF := testobserve.NewFactory(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{
		targetBill: &wallet.Bill{
			Id:    money.NewBillID(nil, []byte{1}),
			Value: 1,
		}})
	defer mockServer.Close()

	// add new key
	_, err := execCommand(logF, homedir, "add-key")
	require.NoError(t, err)

	// verify list bills for specific account only shows given account bills
	stdout, err := execBillsCommand(logF, homedir, "list -k 2 --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	lines := stdout.Lines
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "Account #2")
	require.Contains(t, lines[1], "#1")
}

func TestWalletBillsListCmd_ExtraAccountTotal(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	logF := testobserve.NewFactory(t)

	// add new key
	stdout, err := execCommand(logF, homedir, "add-key")
	require.NoError(t, err)
	pubKey2 := strings.Split(stdout.Lines[0], " ")[3]

	mockServer, addr := mockBackendCalls(&backendMockReturnConf{
		targetBill: &wallet.Bill{
			Id:    money.NewBillID(nil, []byte{1}),
			Value: 1e9,
		},
		customFullPath: "/" + client.ListBillsPath + "?includeDcBills=false&limit=100&pubkey=" + pubKey2,
		customResponse: `{"bills": []}`})
	defer mockServer.Close()

	// verify both accounts are listed
	stdout, err = execBillsCommand(logF, homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1")
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 10")
	verifyStdout(t, stdout, "Account #2 - empty")
}

func TestWalletBillsListCmd_ShowUnswappedFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	logF := testobserve.NewFactory(t)

	// get pub key
	stdout, err := execCommand(logF, homedir, "get-pubkeys")
	require.NoError(t, err)
	pubKey := strings.Split(stdout.Lines[0], " ")[1]

	// verify no -s flag sends includeDcBills=false by default
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{
		customFullPath: "/" + client.ListBillsPath + "?includeDcBills=false&limit=100&pubkey=" + pubKey,
		customResponse: `{"bills": [{"value":"22222222"}]}`})

	stdout, err = execBillsCommand(logF, homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x 0.222'222'22")
	mockServer.Close()

	// verify -s flag sends includeDcBills=true
	mockServer, addr = mockBackendCalls(&backendMockReturnConf{
		customFullPath: "/" + client.ListBillsPath + "?includeDcBills=true&limit=100&pubkey=" + pubKey,
		customResponse: `{"bills": [{"value":"33333333"}]}`})

	stdout, err = execBillsCommand(logF, homedir, "list --alphabill-api-uri "+addr.Host+" -s")
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x 0.333'333'33")
	mockServer.Close()
}

func TestWalletBillsListCmd_ShowLockedBills(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	var billsList []string
	for i := 1; i <= 3; i++ {
		idBase64 := toBase64(money.NewBillID(nil, []byte{byte(i)}))
		billsList = append(billsList, fmt.Sprintf(`{"id":"%s","locked":"%d","value":"100000000"}`, idBase64, i))
	}
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: fmt.Sprintf(`{"bills": [%s]}`, strings.Join(billsList, ","))})
	defer mockServer.Close()
	stdout, err := execBillsCommand(testobserve.NewFactory(t), homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00 (locked for adding fees)")
	verifyStdout(t, stdout, "#2 0x000000000000000000000000000000000000000000000000000000000000000200 1.000'000'00 (locked for reclaiming fees)")
	verifyStdout(t, stdout, "#3 0x000000000000000000000000000000000000000000000000000000000000000300 1.000'000'00 (locked for dust collection)")
}

func TestWalletBillsLockUnlockCmd_Ok(t *testing.T) {
	// create wallet
	am, homedir := createNewWallet(t)
	pubkey, err := am.GetPublicKey(0)
	require.NoError(t, err)
	am.Close()

	// start money partition
	initialBill := &money.InitialBill{
		ID:    defaultInitialBillID,
		Value: 2e8,
		Owner: templates.NewP2pkh256BytesFromKey(pubkey),
	}
	moneyPartition := testutils.CreateMoneyPartition(t, initialBill, 1)
	logF := testobserve.NewFactory(t)
	_ = testutils.StartAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	testutils.StartPartitionRPCServers(t, moneyPartition)

	// start wallet backend
	addr, _ := testutils.StartMoneyBackend(t, moneyPartition, initialBill)

	// create fee credit for txs
	stdout, err := execCommand(logF, homedir, fmt.Sprintf("fees add --alphabill-api-uri %s", addr))
	require.NoError(t, err)

	// lock bill
	stdout, err = execBillsCommand(logF, homedir, fmt.Sprintf("lock --alphabill-api-uri %s --bill-id %s", addr, defaultInitialBillID))
	require.NoError(t, err)
	verifyStdout(t, stdout, "Bill locked successfully.")

	// verify bill locked
	stdout, err = execBillsCommand(logF, homedir, fmt.Sprintf("list --alphabill-api-uri %s", addr))
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00 (manually locked by user)")

	// unlock bill
	stdout, err = execBillsCommand(logF, homedir, fmt.Sprintf("unlock --alphabill-api-uri %s --bill-id %s", addr, defaultInitialBillID))
	require.NoError(t, err)
	verifyStdout(t, stdout, "Bill unlocked successfully.")

	// verify bill unlocked
	stdout, err = execBillsCommand(logF, homedir, fmt.Sprintf("list --alphabill-api-uri %s", addr))
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00")
}

func TestWalletBillsLockUnlockCmd_Nok(t *testing.T) {
	// TODO convert to unit test, no need to start entire network
	// create wallet
	am, homedir := createNewWallet(t)
	pubkey, err := am.GetPublicKey(0)
	require.NoError(t, err)
	am.Close()

	// start money partition
	initialBill := &money.InitialBill{
		ID:    defaultInitialBillID,
		Value: 2e8,
		Owner: templates.NewP2pkh256BytesFromKey(pubkey),
	}
	moneyPartition := testutils.CreateMoneyPartition(t, initialBill, 1)
	logF := testobserve.NewFactory(t)
	_ = testutils.StartAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	testutils.StartPartitionRPCServers(t, moneyPartition)
	testutils.StartPartitionRPCServers(t, moneyPartition)

	// start wallet backend
	addr, _ := testutils.StartMoneyBackend(t, moneyPartition, initialBill)

	// lock bill
	_, err = execBillsCommand(logF, homedir, fmt.Sprintf("lock --alphabill-api-uri %s --bill-id %s", addr, defaultInitialBillID))
	require.ErrorContains(t, err, "not enough fee credit in wallet")

	// unlock bill
	_, err = execBillsCommand(logF, homedir, fmt.Sprintf("unlock --alphabill-api-uri %s --bill-id %s", addr, defaultInitialBillID))
	require.ErrorContains(t, err, "not enough fee credit in wallet")
}

func execBillsCommand(obsF Factory, homeDir, command string) (*testutils.TestConsoleWriter, error) {
	return execCommand(obsF, homeDir, " bills "+command)
}
