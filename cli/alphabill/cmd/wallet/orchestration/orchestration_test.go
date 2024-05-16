package orchestration

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
)

func TestAddVar_OK(t *testing.T) {
	wallets := testutils.SetupWallets(t, 1, 1)
	poaOwnerPredicate := templates.NewP2pkh256BytesFromKey(wallets[0].PubKeys[0])
	orchestrationPartition := testutils.CreateOrchestrationPartition(t, poaOwnerPredicate)
	network := testutils.SetupNetwork(t, wallets[0].PubKeys[0], orchestrationPartition)

	rpcUrl := network.RpcUrl(t, orchestration.DefaultSystemID)

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

	orcCmd := testutils.NewSubCmdExecutor(NewCmd, "--rpc-url", rpcUrl).WithHome(wallets[0].Homedir)

	testutils.VerifyStdout(t, orcCmd.Exec(t, "add-var", "--partition-id", "1", "--var-file", varFile),
		"Validator Assignment Record added successfully.")

	require.Eventually(t, testutils.BlockchainContains(orchestrationPartition, func(tx *types.TransactionOrder) bool {
		if tx.PayloadType() == orchestration.PayloadTypeAddVAR {
			var attrs *orchestration.AddVarAttributes
			require.NoError(t, tx.UnmarshalAttributes(&attrs))
			require.Equal(t, varData, attrs.Var)
			return true
		}
		return false
	}), testutils.WaitDuration, testutils.WaitTick)
}

func writeVarFile(t *testing.T, homedir string, varData orchestration.ValidatorAssignmentRecord) string {
	varFilePath := filepath.Join(homedir, "var-file.json")
	err := util.WriteJsonFile(varFilePath, &varData)
	require.NoError(t, err)
	return varFilePath
}
