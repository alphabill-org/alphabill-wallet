package fees

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

func TestWalletFeesCmds_MoneyPartition(t *testing.T) {
	// start money partition
	homedir, moneyRpcUrl, _ := setupNetwork(t, []*testpartition.NodePartition{})

	// list fees
	stdout, err := execFeesCommand(t, homedir, moneyRpcUrl, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// add fee credits
	amount := uint64(150)
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.Lines[0])

	// verify fee credits
	expectedFees := amount*1e8 - 2
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// add more fee credits
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.Lines[0])

	// verify fee credits
	expectedFees = amount*2*1e8 - 5 // minus 2 for first run, minus 3 for second run
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// reclaim fees
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "reclaim")
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits on money partition.", stdout.Lines[0])

	// list fees
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// add more fees after reclaiming
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.Lines[0])

	// verify list fees
	expectedFees = amount*1e8 - 2
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])
}

func TestWalletFeesCmds_TokenPartition(t *testing.T) {
	// start money and tokens partition
	tokensPartition := testutils.CreateTokensPartition(t)
	homedir, moneyRpcUrl, _ := setupNetwork(t, []*testpartition.NodePartition{tokensPartition})

	tokensRpcUrl := tokensPartition.Nodes[0].AddrRPC
	args := fmt.Sprintf("--partition=tokens --partition-rpc-url %s", tokensRpcUrl)

	// list fees on token partition
	stdout, err := execFeesCommand(t, homedir, moneyRpcUrl, "list "+args)
	require.NoError(t, err)

	require.Equal(t, "Partition: tokens", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// add fee credits
	amount := uint64(150)
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, fmt.Sprintf("add --amount=%d %s", amount, args))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.Lines[0])

	// verify fee credits
	expectedFees := amount*1e8 - 2
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// add more fee credits to token partition
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, fmt.Sprintf("add --amount=%d %s", amount, args))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.Lines[0])

	// verify fee credits to token partition
	expectedFees = amount*2*1e8 - 5
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// reclaim fees
	// invalid transaction: fee credit record unit is nil
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "reclaim "+args)
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits on tokens partition.", stdout.Lines[0])

	// list fees
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.Lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.Lines[1])

	// add more fees after reclaiming
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, fmt.Sprintf("add --amount=%d %s", amount, args))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.Lines[0])

	// verify list fees
	expectedFees = amount*1e8 - 2
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])
}

func TestWalletFeesCmds_MinimumFeeAmount(t *testing.T) {
	homedir, moneyRpcUrl, _ := setupNetwork(t, []*testpartition.NodePartition{})

	// try to add invalid fee amount
	_, err := execFeesCommand(t, homedir, moneyRpcUrl, "add --amount=0.00000003")
	require.Errorf(t, err, "minimum fee credit amount to add is %d", util.AmountToString(fees.MinimumFeeAmount, 8))

	// add minimum fee amount
	stdout, err := execFeesCommand(t, homedir, moneyRpcUrl, "add --amount=0.00000004")
	require.NoError(t, err)
	require.Equal(t, "Successfully created 0.00000004 fee credits on money partition.", stdout.Lines[0])

	// verify fee credit is below minimum
	expectedFees := uint64(2)
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// reclaim with invalid amount
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "reclaim")
	require.Errorf(t, err, "insufficient fee credit balance. Minimum amount is %d", util.AmountToString(fees.MinimumFeeAmount, 8))
	require.Empty(t, stdout.Lines)

	// add more fee credit
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "add --amount=0.00000005")
	require.NoError(t, err)
	require.Equal(t, "Successfully created 0.00000005 fee credits on money partition.", stdout.Lines[0])

	// verify fee credit is valid for reclaim
	expectedFees = uint64(4)
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", util.AmountToString(expectedFees, 8)), stdout.Lines[1])

	// now we have enough credit to reclaim
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "reclaim")
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits on money partition.", stdout.Lines[0])
}

func TestWalletFeesLockCmds_Ok(t *testing.T) {
	homedir, moneyRpcUrl, _ := setupNetwork(t, []*testpartition.NodePartition{})

	// create fee credit bill by adding fee credit
	stdout, err := execFeesCommand(t, homedir, moneyRpcUrl, "add --amount 1")
	require.NoError(t, err)
	require.Equal(t, "Successfully created 1 fee credits on money partition.", stdout.Lines[0])

	// lock fee credit record
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "lock -k 1")
	require.NoError(t, err)
	require.Equal(t, "Fee credit record locked successfully.", stdout.Lines[0])

	// verify fee credit bill locked
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, "Account #1 0.999'999'97 (manually locked by user)", stdout.Lines[1])

	// unlock fee credit record
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "unlock -k 1")
	require.NoError(t, err)
	require.Equal(t, "Fee credit record unlocked successfully.", stdout.Lines[0])

	// list fees
	stdout, err = execFeesCommand(t, homedir, moneyRpcUrl, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.Lines[0])
	require.Equal(t, "Account #1 0.999'999'96", stdout.Lines[1])
}

func execFeesCommand(t *testing.T, homeDir, moneyRpcUrl, command string) (*testutils.TestConsoleWriter, error) {
	outputWriter := &testutils.TestConsoleWriter{}
	wcmd := NewFeesCmd(&types.WalletConfig{
		Base:          &types.BaseConfiguration{HomeDir: homeDir, Logger: logger.New(t), ConsoleWriter: outputWriter},
		WalletHomeDir: filepath.Join(homeDir, "wallet"),
	})
	args := strings.Split(command, " ")
	args = append(args, "-r", moneyRpcUrl)
	wcmd.SetArgs(args)
	return outputWriter, wcmd.Execute()
}

// setupNetwork starts alphabill network.
// Starts money partition, and optionally any other partitions, with rpc servers up and running.
// The initial bill is set to the created wallet.
// Returns wallet homedir, money node url and reference to the network object.
func setupNetwork(t *testing.T, otherPartitions []*testpartition.NodePartition) (string, string, *testpartition.AlphabillNetwork) {
	// create wallet
	am, homedir := testutils.CreateNewWallet(t)
	defer am.Close()
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	genesisConfig := &testutil.MoneyGenesisConfig{
		InitialBillID:      testutils.DefaultInitialBillID,
		InitialBillValue:   1e18,
		InitialBillOwner:   templates.NewP2pkh256BytesFromKey(accountKey.PubKey),
		DCMoneySupplyValue: 10000,
	}
	rpcUrl, abNet := testutils.SetupNetwork(t, genesisConfig, otherPartitions)
	return homedir, rpcUrl, abNet
}
