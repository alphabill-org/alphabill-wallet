package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	sdkorchestration "github.com/alphabill-org/alphabill-go-sdk/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/util"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/orchestration"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	clitypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

func TestAddVar_OK(t *testing.T) {
	network := startOrchestrationPartition(t)
	orchestrationPartition, err := network.abNetwork.GetNodePartition(sdkorchestration.DefaultSystemID)
	require.NoError(t, err)
	rpcUrl := orchestrationPartition.Nodes[0].AddrRPC
	varData := sdkorchestration.ValidatorAssignmentRecord{
		EpochNumber:            0,
		EpochSwitchRoundNumber: 10000,
		ValidatorAssignment: sdkorchestration.ValidatorAssignment{
			Validators: []sdkorchestration.ValidatorInfo{
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
	varFile := writeVarFile(t, network.homeDir, varData)

	stdout, err := execOrchestrationCmd(t, network.homeDir, fmt.Sprintf("add-var -r %s --partition-id %d --var-file %s", rpcUrl, 1, varFile))
	require.NoError(t, err)
	require.Equal(t, "Validator Assignment Record added successfully.", stdout.Lines[0])
	require.Eventually(t, testpartition.BlockchainContains(orchestrationPartition, func(tx *types.TransactionOrder) bool {
		if tx.PayloadType() == sdkorchestration.PayloadTypeAddVAR {
			var attrs *sdkorchestration.AddVarAttributes
			require.NoError(t, tx.UnmarshalAttributes(&attrs))
			require.Equal(t, varData, attrs.Var)
			return true
		}
		return false
	}), test.WaitDuration, test.WaitTick)
}

func writeVarFile(t *testing.T, homedir string, varData sdkorchestration.ValidatorAssignmentRecord) string {
	varFilePath := filepath.Join(homedir, "var-file.json")
	err := util.WriteJsonFile(varFilePath, &varData)
	require.NoError(t, err)
	return varFilePath
}

func startOrchestrationPartition(t *testing.T) *orchestrationNetwork {
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	observe := testobserve.NewFactory(t)
	log := observe.DefaultLogger()

	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	defer am.Close()
	require.NoError(t, am.CreateKeys(""))
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	poaOwnerPredicate := templates.NewP2pkh256BytesFromKey(accountKey.PubKey)
	orchestrationPartition := createOrchestrationPartition(t, poaOwnerPredicate)
	abNet := testutils.StartAlphabill(t, []*testpartition.NodePartition{orchestrationPartition})
	testutils.StartRpcServers(t, orchestrationPartition)
	orchestrationRpcClient, err := rpc.DialContext(ctx, args.BuildRpcUrl(orchestrationPartition.Nodes[0].AddrRPC))
	require.NoError(t, err)

	return &orchestrationNetwork{
		abNetwork:              abNet,
		orchestrationRpcClient: orchestrationRpcClient,
		homeDir:                homeDir,
		walletHomeDir:          walletDir,
		walletAccountKey:       accountKey,
		log:                    log,
		ctx:                    ctx,
	}
}

func createOrchestrationPartition(t *testing.T, ownerPredicate types.PredicateBytes) *testpartition.NodePartition {
	s := state.NewEmptyState()
	network, err := testpartition.NewPartition(
		t,
		"orchestration node",
		1,
		func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
			s = s.Clone()
			txSystem, err := orchestration.NewTxSystem(
				testobserve.Default(t),
				orchestration.WithState(s),
				orchestration.WithTrustBase(tb),
				orchestration.WithOwnerPredicate(ownerPredicate),
			)
			require.NoError(t, err)
			return txSystem
		},
		sdkorchestration.DefaultSystemID,
		s,
	)
	require.NoError(t, err)
	return network
}

type orchestrationNetwork struct {
	abNetwork              *testpartition.AlphabillNetwork
	orchestrationRpcClient *rpc.Client

	homeDir          string
	walletHomeDir    string
	walletAccountKey *account.AccountKey
	log              *slog.Logger
	ctx              context.Context
}

func execOrchestrationCmd(t *testing.T, homedir string, command string) (*testutils.TestConsoleWriter, error) {
	outputWriter := &testutils.TestConsoleWriter{}
	ccmd := NewCmd(&clitypes.WalletConfig{
		Base: &clitypes.BaseConfiguration{
			HomeDir:       homedir,
			ConsoleWriter: outputWriter,
			Logger:        logger.New(t),
		},
		WalletHomeDir: filepath.Join(homedir, "wallet"),
	})
	ccmd.SetArgs(strings.Split(command, " "))
	return outputWriter, ccmd.Execute()
}
