//go:build !nodocker

package wallet

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	baseutil "github.com/alphabill-org/alphabill-go-base/util"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/util"
)

/*
Prep: start money partition, send initial bill to wallet-1
Test scenario 1: wallet-1 sends two transactions to wallet-2
Test scenario 1.1: wallet-2 sends transactions back to wallet-1
Test scenario 2: wallet-1 account 1 sends two transactions to wallet-1 account 2
Test scenario 2.1: wallet-1 account 2 sends one transaction to wallet-1 account 3
Test scenario 3: wallet-1 sends tx without confirming
*/
func TestSendingMoneyUsingWallets_integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t)

	walletCmd := newWalletCmdExecutor("--rpc-url", abNet.MoneyRpcUrl).WithHome(wallets[0].Homedir)

	// create fee credit for wallet-1
	feeAmountAlpha := uint64(1)
	addFeeCredit(t, wallets[0].Homedir, feeAmountAlpha, "money", abNet.MoneyRpcUrl, abNet.MoneyRpcUrl)

	// verify fee credit received
	w1BalanceBilly := 1e18 - feeAmountAlpha*1e8
	waitForFeeCreditCLI(t, walletCmd, feeAmountAlpha*1e8-2, 0)

	// TS1:
	// send two transactions to wallet-2
	stdout := walletCmd.Exec(t, "send", "--amount", "50", "--address", fmt.Sprintf("0x%x", wallets[1].PubKeys[0]))
	testutils.VerifyStdout(t, stdout,
		"Successfully confirmed transaction(s)",
		"Paid 0.000'000'02 fees for transaction(s)")

	// verify wallet-1 balance is decreased
	w1BalanceBilly -= 50 * 1e8
	testutils.VerifyStdoutEventually(t, walletCmd.ExecFunc(t, "get-balance"),
		fmt.Sprintf("#%d %s", 1, util.AmountToString(w1BalanceBilly, 8)))

	// verify wallet-2 received said bills
	w2BalanceBilly := uint64(50 * 1e8)
	testutils.VerifyStdoutEventually(t, walletCmd.WithHome(wallets[1].Homedir).ExecFunc(t, "get-balance"),
		fmt.Sprintf("#%d %s", 1, util.AmountToString(w2BalanceBilly, 8)))

	// TS1.2: send bills back to wallet-1
	// create fee credit for wallet-2
	addFeeCredit(t, wallets[1].Homedir, feeAmountAlpha, "money", abNet.MoneyRpcUrl, abNet.MoneyRpcUrl)

	// verify fee credit received for wallet-2
	w2BalanceBilly = w2BalanceBilly - feeAmountAlpha*1e8
	waitForFeeCreditCLI(t, walletCmd.WithHome(wallets[1].Homedir), feeAmountAlpha*1e8-2, 0)

	// send wallet-2 bills back to wallet-1
	stdout = walletCmd.WithHome(wallets[1].Homedir).Exec(t,
		"send",
		"--amount", util.AmountToString(w2BalanceBilly, 8),
		"--address", hexutil.Encode(wallets[0].PubKeys[0]))
	testutils.VerifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-2 balance is reduced
	waitForBalanceCLI(t, walletCmd.WithHome(wallets[1].Homedir), 0, 0)

	// verify wallet-1 balance is increased
	w1BalanceBilly += w2BalanceBilly
	waitForBalanceCLI(t, walletCmd, w1BalanceBilly, 0)

	// TS2:
	// create w1k3
	pubKey3Hex := addAccount(t, wallets[0].Homedir)

	// send two bills to w1k2
	stdout = walletCmd.Exec(t,
		"send",
		"--key", "1",
		"--amount", "50,150",
		"--address", fmt.Sprintf("0x%X,0x%X", wallets[0].PubKeys[1], wallets[0].PubKeys[1]))
	testutils.VerifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify w1k1 balance is decreased
	w1BalanceBilly -= 200 * 1e8
	testutils.VerifyStdoutEventually(t, walletCmd.ExecFunc(t, "get-balance", "--key", "1"),
		fmt.Sprintf("#%d %s", 1, util.AmountToString(w1BalanceBilly, 8)))

	// verify w1k2 received said bills
	acc2BalanceBilly := uint64(200 * 1e8)
	testutils.VerifyStdoutEventually(t, walletCmd.ExecFunc(t, "get-balance", "--key", "2"),
		fmt.Sprintf("#%d %s", 2, util.AmountToString(acc2BalanceBilly, 8)))

	// TS2.1:
	// create fee credit for account 2
	stdout = walletCmd.Exec(t, "fees", "add", "--amount", strconv.FormatUint(feeAmountAlpha, 10), "--key", "2")

	testutils.VerifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	waitForFeeCreditCLI(t, walletCmd, feeAmountAlpha*1e8-2, 1)

	// send tx from account-2 to account-3
	stdout = walletCmd.Exec(t, "send", "--amount", "100", "--key", "2", "--address", pubKey3Hex)
	testutils.VerifyStdout(t, stdout, "Successfully confirmed transaction(s)")
	waitForBalanceCLI(t, walletCmd, 100*1e8, 2)

	// verify account-2 fcb balance is reduced after send
	stdout = walletCmd.Exec(t, "fees", "list", "--key", "2")
	acc2FeeCredit := feeAmountAlpha*1e8 - 4 // minus two for tx and minus two for creating fee credit
	acc2FeeCreditString := util.AmountToString(acc2FeeCredit, 8)
	testutils.VerifyStdout(t, stdout, fmt.Sprintf("Account #2 %s", acc2FeeCreditString))

	// TS3:
	// verify transaction is broadcast immediately without confirmation
	stdout = walletCmd.Exec(t, "send", "-w", "false", "--amount", "2", "--address", fmt.Sprintf("0x%X", wallets[1].PubKeys[1]))
	testutils.VerifyStdout(t, stdout, "Successfully sent transaction(s)")
}

/*
Test scenario:

	w1k1 sends two bills to w1k2 and w1k3
	w1 runs dust collection
	w1k2 and w1k3 should have only single bill
*/
func TestCollectDustInMultiAccountWallet(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t)
	walletCmd := newWalletCmdExecutor("--rpc-url", abNet.MoneyRpcUrl).WithHome(wallets[0].Homedir)

	// add fee credit for w1k1
	walletCmd.Exec(t, "fees", "add",
		"--amount", "1",
		"--partition-rpc-url", abNet.MoneyRpcUrl)

	pubKey2Hex := hexutil.Encode(wallets[0].PubKeys[1])
	pubKey3Hex := addAccount(t, wallets[0].Homedir)

	// send two bills to both w1k2 and w1k3
	stdout := walletCmd.Exec(t, "send",
		"--amount", "10,10,10,10",
		"--address", fmt.Sprintf("%s,%s,%s,%s", pubKey2Hex, pubKey2Hex, pubKey3Hex, pubKey3Hex))
	testutils.VerifyStdout(t, stdout,
		"Successfully confirmed transaction(s)",
		"Paid 0.000'000'02 fees for transaction(s)")

	walletCmd.Exec(t, "fees", "add",
		"--key", "2",
		"--amount", "1",
		"--partition-rpc-url", abNet.MoneyRpcUrl)

	walletCmd.Exec(t, "fees", "add",
		"--key", "3",
		"--amount", "1",
		"--partition-rpc-url", abNet.MoneyRpcUrl)

	walletCmd.Exec(t, "collect-dust", "--key", "0")

	// Verify that w1k2 has a single bill with value 19
	testutils.VerifyStdout(t, walletCmd.Exec(t, "bills", "list", "--key", "2"),
		util.AmountToString(19*1e8, 8))

	// Verify that w1k3 has a single bill with value 19
	testutils.VerifyStdout(t, walletCmd.Exec(t, "bills", "list", "--key", "3"),
		util.AmountToString(19*1e8, 8))
}

func TestWalletBillsLockUnlockCmd_Ok(t *testing.T) {
	// setup network
	wallets, abNet := testutils.SetupNetworkWithWallets(t)

	walletCmd := newWalletCmdExecutor("--rpc-url", abNet.MoneyRpcUrl).WithHome(wallets[0].Homedir)

	// add fee credit
	stdout := walletCmd.Exec(t, "fees", "add", "--amount=1")
	require.Equal(t, "Successfully created 1 fee credits on money partition.", stdout.Lines[0])

	// lock bill
	stdout = walletCmd.Exec(t, "bills", "lock", "--bill-id", money.NewBillID(nil, []byte{1}).String())
	testutils.VerifyStdout(t, stdout, "Bill locked successfully.")

	// verify bill locked
	stdout = walletCmd.Exec(t, "bills", "list")
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000101 9'999'999'999.000'000'00 (manually locked by user)")

	// unlock bill
	stdout = walletCmd.Exec(t, "bills", "unlock", "--bill-id", money.NewBillID(nil, []byte{1}).String())
	testutils.VerifyStdout(t, stdout, "Bill unlocked successfully.")

	// verify bill unlocked
	stdout = walletCmd.Exec(t, "bills", "list")
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000101 9'999'999'999.000'000'00")
}

func TestWalletPermissionedAddCreditCmd_Ok(t *testing.T) {
	// setup network
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithEnterpriseTokensNode(t))

	walletCmd := newWalletCmdExecutor("--rpc-url", abNet.EnterpriseTokensRpcUrl).WithHome(wallets[0].Homedir)
	targetPubkey := wallets[0].PubKeys[1]

	// add fee credit
	testutils.VerifyStdout(t,
		walletCmd.Exec(t, "permissioned", "add-credit", "--target-pubkey", fmt.Sprintf("0x%x", targetPubkey), "--amount", "9"),
		"Fee credit added successfully")

	testutils.VerifyStdout(t,
		walletCmd.Exec(t, "fees", "list", "--partition", "enterprise-tokens", "--partition-rpc-url", abNet.EnterpriseTokensRpcUrl),
		"Account #2 9.000'000'00")

	// add more fee credit
	testutils.VerifyStdout(t,
		walletCmd.Exec(t, "permissioned", "add-credit", "--target-pubkey", fmt.Sprintf("0x%x", targetPubkey), "--amount", "7"),
		"Fee credit added successfully")

	testutils.VerifyStdout(t,
		walletCmd.Exec(t, "fees", "list", "--partition", "enterprise-tokens", "--partition-rpc-url", abNet.EnterpriseTokensRpcUrl),
		"Account #2 16.000'000'00")

	// delete fee credit
	testutils.VerifyStdout(t,
		walletCmd.Exec(t, "permissioned", "delete-credit", "--target-pubkey", fmt.Sprintf("0x%x", targetPubkey)),
		"Fee credit deleted successfully")

	testutils.VerifyStdout(t,
		walletCmd.Exec(t, "fees", "list", "--partition", "enterprise-tokens", "--partition-rpc-url", abNet.EnterpriseTokensRpcUrl),
		"Account #2 0.000'000'00")
}

func TestOrchestration_AddVarOK(t *testing.T) {
	wallets, net := testutils.SetupNetworkWithWallets(t, testutils.WithOrchestrationNode(t))

	varData := orchestration.ValidatorAssignmentRecord{
		EpochNumber:            0,
		EpochSwitchRoundNumber: 10000,
		ValidatorAssignment: orchestration.ValidatorAssignment{
			Validators: []orchestration.ValidatorInfo{
				{
					ValidatorID: []byte{1},
					Stake:       100,
				},
				{
					ValidatorID: []byte{2},
					Stake:       100,
				},
				{
					ValidatorID: []byte{3},
					Stake:       100,
				},
			},
			QuorumSize: 150,
		},
	}
	varFile := writeVarFile(t, wallets[0].Homedir, varData)

	orcCmd := newWalletCmdExecutor("orchestration", "--rpc-url", net.OrchestrationRpcUrl).WithHome(wallets[0].Homedir)

	testutils.VerifyStdout(t, orcCmd.Exec(t, "add-var", "--partition-id", "1", "--var-file", varFile),
		"Validator Assignment Record added successfully.")

	// TODO: verify with some kind of list-var command?
	// require.Eventually(t, testutils.BlockchainContains(orchestrationPartition, func(tx *types.TransactionOrder) bool {
	// 	if tx.PayloadType() == orchestration.PayloadTypeAddVAR {
	// 		var attrs *orchestration.AddVarAttributes
	// 		require.NoError(t, tx.UnmarshalAttributes(&attrs))
	// 		require.Equal(t, varData, attrs.Var)
	// 		return true
	// 	}
	// 	return false
	// }), testutils.WaitDuration, testutils.WaitTick)
}

func writeVarFile(t *testing.T, homedir string, varData orchestration.ValidatorAssignmentRecord) string {
	varFilePath := filepath.Join(homedir, "var-file.json")
	err := baseutil.WriteJsonFile(varFilePath, &varData)
	require.NoError(t, err)
	return varFilePath
}

func waitForBalanceCLI(t *testing.T, walletCmd *testutils.CmdExecutor, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := walletCmd.Exec(t, "get-balance")
		for _, line := range stdout.Lines {
			expectedBalanceStr := util.AmountToString(expectedBalance, 8)
			if line == fmt.Sprintf("#%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, testutils.WaitDuration, testutils.WaitTick)
}

func waitForFeeCreditCLI(t *testing.T, walletCmd *testutils.CmdExecutor, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := walletCmd.Exec(t, "fees", "list")
		for _, line := range stdout.Lines {
			expectedBalanceStr := util.AmountToString(expectedBalance, 8)
			if line == fmt.Sprintf("Account #%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, testutils.WaitDuration, testutils.WaitTick)
}

// addAccount calls "add-key" cli function on given wallet and returns the added pubkey hex
func addAccount(t *testing.T, home string) string {
	stdout := newWalletCmdExecutor().WithHome(home).Exec(t, "add-key")
	for _, line := range stdout.Lines {
		if strings.HasPrefix(line, "Added key #") {
			return line[13:]
		}
	}
	return ""
}
