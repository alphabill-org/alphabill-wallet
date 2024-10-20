//go:build !nodocker

package wallet

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/util"
)

func TestWalletFeesCmds_MoneyPartition(t *testing.T) {
	// start money partition
	wallets, abNet := testutils.SetupNetworkWithWallets(t)

	feesCmd := newWalletCmdExecutor("fees", "--rpc-url", abNet.MoneyRpcUrl).WithHome(wallets[0].Homedir)

	// list fees for all accounts (explicitly specifying accNr 0 lists all)
	stdout := feesCmd.Exec(t, "list", "--key", "0")
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// list fees for specific account
	stdout = feesCmd.Exec(t, "list", "--key", "1")
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// add fee credits
	amount := uint64(150)
	stdout = feesCmd.Exec(t, "add", "--amount", strconv.FormatUint(amount, 10))
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.Lines[0])

	// verify fee credits
	expectedFees := amount*1e8 - 2
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// add more fee credits
	stdout = feesCmd.Exec(t, "add", "--amount", strconv.FormatUint(amount, 10))
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.Lines[0])

	// verify fee credits
	expectedFees = amount*2*1e8 - 5 // minus 2 for first run, minus 3 for second run
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// reclaim fees
	stdout = feesCmd.Exec(t, "reclaim")
	require.Equal(t, "Successfully reclaimed fee credits on money partition.", stdout.Lines[0])

	// list fees
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// add more fees after reclaiming
	stdout = feesCmd.Exec(t, "add", "--amount", strconv.FormatUint(amount, 10))
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.Lines[0])

	// verify list fees
	expectedFees = amount*1e8 - 2
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])
}

func TestWalletFeesCmds_TokenPartition(t *testing.T) {
	// start money and tokens partition
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	feesCmd := newWalletCmdExecutor("fees",
		"--rpc-url", abNet.MoneyRpcUrl,
		"--partition", "tokens",
		"--partition-rpc-url", abNet.TokensRpcUrl).WithHome(wallets[0].Homedir)

	// list fees on token partition
	stdout := feesCmd.Exec(t, "list")

	require.Equal(t, "Partition: tokens", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// add fee credits
	amount := uint64(150)
	stdout = feesCmd.Exec(t, "add", "--amount", strconv.FormatUint(amount, 10))
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.Lines[0])

	// verify fee credits
	expectedFees := amount*1e8 - 2
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: tokens", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// add more fee credits to token partition
	stdout = feesCmd.Exec(t, "add", "--amount", strconv.FormatUint(amount, 10))
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.Lines[0])

	// verify fee credits to token partition
	expectedFees = amount*2*1e8 - 5
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: tokens", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// reclaim fees
	stdout = feesCmd.Exec(t, "reclaim")
	require.Equal(t, "Successfully reclaimed fee credits on tokens partition.", stdout.Lines[0])

	// list fees
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: tokens", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// add more fees after reclaiming
	stdout = feesCmd.Exec(t, "add", "--amount", strconv.FormatUint(amount, 10))
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.Lines[0])

	// verify list fees
	expectedFees = amount*1e8 - 2
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: tokens", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// manage fee credits as if it were a permissioned partition
	permissionedCmd := newWalletCmdExecutor("permissioned",
		"--rpc-url", abNet.TokensRpcUrl,
		"--target-pubkey", "0x01").WithHome(wallets[0].Homedir)

	permissionedCmd.ExecWithError(t, "cannot add fee credit, partition not in permissioned mode", "add-credit", "--amount", "10")
	permissionedCmd.ExecWithError(t, "cannot delete fee credit, partition not in permissioned mode", "delete-credit")
}

func TestWalletFeesCmds_EvmPartition(t *testing.T) {
	// start money and EVM partition
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithEvmNode(t))

	feesCmd := newWalletCmdExecutor("fees",
		"--rpc-url", abNet.MoneyRpcUrl,
		"--partition", "evm",
		"--partition-rpc-url", abNet.EvmRpcUrl).WithHome(wallets[0].Homedir)

	// list fees on EVM partition
	stdout := feesCmd.Exec(t, "list")

	require.Equal(t, "Partition: evm", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// add fee credits
	amount := uint64(150)
	stdout = feesCmd.Exec(t, "add", "--amount", strconv.FormatUint(amount, 10))
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on evm partition.", amount), stdout.Lines[0])

	// verify fee credits
	expectedFees := amount*1e8 - 2
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: evm", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// add more fee credits to EVM partition
	stdout = feesCmd.Exec(t, "add", "--amount", strconv.FormatUint(amount, 10))
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on evm partition.", amount), stdout.Lines[0])

	// verify fee credits to EVM partition
	expectedFees = amount*2*1e8 - 4
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: evm", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// reclaim fees
	stdout = feesCmd.Exec(t, "reclaim")
	require.Equal(t, "Successfully reclaimed fee credits on evm partition.", stdout.Lines[0])

	// list fees
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: evm", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// add more fees after reclaiming
	stdout = feesCmd.Exec(t, "add", "--amount", strconv.FormatUint(amount, 10))
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on evm partition.", amount), stdout.Lines[0])

	// verify list fees
	expectedFees = amount*1e8 - 2
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: evm", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])
}

func TestWalletFeesCmds_MinimumFeeAmount(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t)

	feesCmd := newWalletCmdExecutor("fees",
		"--rpc-url", abNet.MoneyRpcUrl).WithHome(wallets[0].Homedir)

	// try to add invalid fee amount
	maxFee := uint64(5)
	err := fmt.Sprintf("minimum fee credit amount to add is %s", util.AmountToString(2*maxFee+1, 8))
	feesCmd.ExecWithError(t, err, "add", "--amount", "0.00000010", "--max-fee", strconv.FormatUint(maxFee, 10))

	// add minimum fee amount
	stdout := feesCmd.Exec(t, "add", "--amount=0.00000011", "--max-fee", strconv.FormatUint(maxFee, 10))
	require.Equal(t, "Successfully created 0.00000011 fee credits on money partition.", stdout.Lines[0])

	// verify fee credit is below minimum
	expectedFees := uint64(9)
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// reclaim with invalid amount
	err = fmt.Sprintf("insufficient fee credit balance. Minimum amount is %s", util.AmountToString(2*maxFee+1, 8))
	feesCmd.ExecWithError(t, err, "reclaim", "--max-fee", strconv.FormatUint(maxFee, 10))

	// add more fee credit
	stdout = feesCmd.Exec(t, "add", "--amount=0.00000045", "--max-fee", strconv.FormatUint(maxFee, 10))
	require.Equal(t, "Successfully created 0.00000045 fee credits on money partition.", stdout.Lines[0])

	// verify fee credit is valid for reclaim
	expectedFees = uint64(51) // 9 - 1 (lockFC) + 45 - 1 (transFC) - 1 (addFC) = 51
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// now we have enough credit to reclaim
	stdout = feesCmd.Exec(t, "reclaim")
	require.Equal(t, "Successfully reclaimed fee credits on money partition.", stdout.Lines[0])
}

func TestWalletFeesLockCmds_Ok(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t)

	feesCmd := newWalletCmdExecutor("fees", "--rpc-url", abNet.MoneyRpcUrl).WithHome(wallets[0].Homedir)

	// create fee credit bill by adding fee credit
	stdout := feesCmd.Exec(t, "add", "--amount", "1")
	require.Equal(t, "Successfully created 1 fee credits on money partition.", stdout.Lines[0])

	// lock fee credit record
	stdout = feesCmd.Exec(t, "lock", "--key", "1")
	require.Equal(t, "Fee credit record locked successfully.", stdout.Lines[0])

	// verify fee credit bill locked
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, "Account #1 0.999'999'97 (manually locked by user)", stdout.Lines[1])

	// unlock fee credit record
	stdout = feesCmd.Exec(t, "unlock", "--key", "1")
	require.Equal(t, "Fee credit record unlocked successfully.", stdout.Lines[0])

	// list fees
	stdout = feesCmd.Exec(t, "list")
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, "Account #1 0.999'999'96", stdout.Lines[1])
}
