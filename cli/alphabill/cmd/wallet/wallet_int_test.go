package wallet

import (
	"context"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

/*
Prep: start network and money backend, send initial bill to wallet-1
Test scenario 1: wallet-1 sends two transactions to wallet-2
Test scenario 1.1: wallet-2 sends transactions back to wallet-1
Test scenario 2: wallet-1 account 1 sends two transactions to wallet-1 account 2
Test scenario 2.1: wallet-1 account 2 sends one transaction to wallet-1 account 3
Test scenario 3: wallet-1 sends tx without confirming
*/
func TestSendingMoneyUsingWallets_integration(t *testing.T) {
	// create 2 wallets
	am1, homedir1 := testutils.CreateNewWallet(t)
	w1AccKey, err := am1.GetAccountKey(0)
	require.NoError(t, err)
	am1.Close()

	am2, homedir2 := testutils.CreateNewWallet(t)
	w2PubKey, err := am2.GetPublicKey(0)
	require.NoError(t, err)
	am2.Close()

	genesisConfig := &testutil.MoneyGenesisConfig{
		InitialBillID:      testutils.DefaultInitialBillID,
		InitialBillValue:   1e18,
		InitialBillOwner:   templates.NewP2pkh256BytesFromKey(w1AccKey.PubKey),
		DCMoneySupplyValue: 10000,
	}
	moneyPartition := testutils.CreateMoneyPartition(t, genesisConfig, 1)
	logF := testobserve.NewFactory(t)
	_ = testutils.StartAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	testutils.StartPartitionRPCServers(t, moneyPartition)

	// start wallet backend
	moneyBackendURL, moneyRestClient := testutils.StartMoneyBackend(t, moneyPartition, genesisConfig)

	// create fee credit for wallet-1
	feeAmountAlpha := uint64(1)
	stdout, err := execCommand(logF, homedir1, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s", feeAmountAlpha, moneyBackendURL))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	w1BalanceBilly := genesisConfig.InitialBillValue - feeAmountAlpha*1e8
	waitForFeeCreditCLI(t, logF, homedir1, moneyBackendURL, feeAmountAlpha*1e8-2, 0)

	// TS1:
	// send two transactions to wallet-2
	stdout, err = execCommand(logF, homedir1, fmt.Sprintf("send --amount 50 --address 0x%x --alphabill-api-uri %s", w2PubKey, moneyBackendURL))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout,
		"Successfully confirmed transaction(s)",
		"Paid 0.000'000'01 fees for transaction(s)")

	// verify wallet-1 balance is decreased
	w1BalanceBilly -= 50 * 1e8
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		consoleWriter, err := execCommand(logF, homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", moneyBackendURL))
		require.NoError(t, err)
		return consoleWriter
	}, fmt.Sprintf("#%d %s", 1, util.AmountToString(w1BalanceBilly, 8)))

	// verify wallet-2 received said bills
	w2BalanceBilly := uint64(50 * 1e8)
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		consoleWriter, err := execCommand(logF, homedir2, fmt.Sprintf("get-balance --alphabill-api-uri %s", moneyBackendURL))
		require.NoError(t, err)
		return consoleWriter
	}, fmt.Sprintf("#%d %s", 1, util.AmountToString(w2BalanceBilly, 8)))

	// TS1.2: send bills back to wallet-1
	// create fee credit for wallet-2
	stdout, err = execCommand(logF, homedir2, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s", feeAmountAlpha, moneyBackendURL))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received for wallet-2
	w2BalanceBilly = w2BalanceBilly - feeAmountAlpha*1e8
	waitForFeeCreditCLI(t, logF, homedir2, moneyBackendURL, feeAmountAlpha*1e8-2, 0)

	// send wallet-2 bills back to wallet-1
	stdout, err = execCommand(logF, homedir2, fmt.Sprintf("send --amount %s --address %s --alphabill-api-uri %s", util.AmountToString(w2BalanceBilly, 8), hexutil.Encode(w1AccKey.PubKey), moneyBackendURL))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-2 balance is reduced
	waitForBalanceCLI(t, logF, homedir2, moneyBackendURL, 0, 0)

	// verify wallet-1 balance is increased
	w1BalanceBilly += w2BalanceBilly
	waitForBalanceCLI(t, logF, homedir1, moneyBackendURL, w1BalanceBilly, 0)

	// TS2:
	// add additional accounts to wallet 1
	pubKey2Hex := addAccount(t, logF, homedir1)
	pubKey3Hex := addAccount(t, logF, homedir1)

	// send two bills to wallet account 2
	stdout, err = execCommand(logF, homedir1, fmt.Sprintf("send -k 1 --amount 50,150 --address %s,%s --alphabill-api-uri %s", pubKey2Hex, pubKey2Hex, moneyBackendURL))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-1 account-1 balance is decreased
	w1BalanceBilly -= 200 * 1e8
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		consoleWriter, err := execCommand(logF, homedir1, fmt.Sprintf("get-balance -k 1 --alphabill-api-uri %s", moneyBackendURL))
		require.NoError(t, err)
		return consoleWriter
	}, fmt.Sprintf("#%d %s", 1, util.AmountToString(w1BalanceBilly, 8)))

	// verify wallet-1 account-2 received said bills
	acc2BalanceBilly := uint64(200 * 1e8)
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		consoleWriter, err := execCommand(logF, homedir1, fmt.Sprintf("get-balance -k 2 --alphabill-api-uri %s", moneyBackendURL))
		require.NoError(t, err)
		return consoleWriter
	}, fmt.Sprintf("#%d %s", 2, util.AmountToString(acc2BalanceBilly, 8)))

	// TS2.1:
	// create fee credit for account 2
	stdout, err = execCommand(logF, homedir1, fmt.Sprintf("fees add --amount %d -k 2 -r %s", feeAmountAlpha, moneyBackendURL))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	waitForFeeCreditCLI(t, logF, homedir1, moneyBackendURL, feeAmountAlpha*1e8-2, 1)

	// send tx from account-2 to account-3
	stdout, err = execCommand(logF, homedir1, fmt.Sprintf("send --amount 100 --key 2 --address %s -r %s", pubKey3Hex, moneyBackendURL))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Successfully confirmed transaction(s)")
	waitForBalanceCLI(t, logF, homedir1, moneyBackendURL, 100*1e8, 2)

	// verify account-2 fcb balance is reduced after send
	stdout, err = execCommand(logF, homedir1, fmt.Sprintf("fees list -k 2 -r %s", moneyBackendURL))
	require.NoError(t, err)
	acc2FeeCredit := feeAmountAlpha*1e8 - 3 // minus one for tx and minus one for creating fee credit
	acc2FeeCreditString := util.AmountToString(acc2FeeCredit, 8)
	testutils.VerifyStdout(t, stdout, fmt.Sprintf("Account #2 %s", acc2FeeCreditString))

	// TS3:
	// verify transaction is broadcast immediately without confirmation
	stdout, err = execCommand(logF, homedir1, fmt.Sprintf("send -w false --amount 2 --address %s --alphabill-api-uri %s", pubKey2Hex, moneyBackendURL))
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "Successfully sent transaction(s)")

	w1TxHistory, _, err := moneyRestClient.GetTxHistory(context.Background(), w1AccKey.PubKey, "", 0)
	require.NoError(t, err)
	require.NotNil(t, w1TxHistory)
	require.Len(t, w1TxHistory, 5)

	w2TxHistory, _, err := moneyRestClient.GetTxHistory(context.Background(), w2PubKey, "", 0)
	require.NoError(t, err)
	require.NotNil(t, w2TxHistory)
	require.Len(t, w2TxHistory, 2)
}

func waitForBalanceCLI(t *testing.T, logF Factory, homedir string, url string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout, err := execCommand(logF, homedir, "get-balance --alphabill-api-uri "+url)
		require.NoError(t, err)
		for _, line := range stdout.Lines {
			expectedBalanceStr := util.AmountToString(expectedBalance, 8)
			if line == fmt.Sprintf("#%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func waitForFeeCreditCLI(t *testing.T, logF Factory, homedir string, url string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout, err := execCommand(logF, homedir, "fees list --alphabill-api-uri "+url)
		require.NoError(t, err)
		for _, line := range stdout.Lines {
			expectedBalanceStr := util.AmountToString(expectedBalance, 8)
			if line == fmt.Sprintf("Account #%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}
