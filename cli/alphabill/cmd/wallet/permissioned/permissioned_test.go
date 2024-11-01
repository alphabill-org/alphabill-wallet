package permissioned

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	"github.com/alphabill-org/alphabill-wallet/client/types"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/stretchr/testify/require"
)

func TestAddFeeCreditCmd(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	as := mocksrv.NewAdminServiceMock(mocksrv.WithInfoResponse(
		&types.NodeInfoResponse{
			NetworkID:        1,
			PartitionID:      50,
			Name:             "tokens",
			PermissionedMode: true,
		}))
	ss := mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t),
			&sdktypes.Unit[any]{
				UnitID: tokens.NewFeeCreditRecordID(nil, []byte{1}),
				Data:   fc.FeeCreditRecord{Balance: 3, OwnerPredicate: nil},
			}))
	rpcUrl := mocksrv.StartServer(t, map[string]interface{}{
		"admin": as,
		"state": ss,
	})

	permissionedCmd := testutils.NewSubCmdExecutor(NewCmd, "--rpc-url", rpcUrl).WithHome(homedir)
	permissionedCmd.ExecWithError(t, "required flag(s)", "add-credit")

	targetPubkey := []byte{1, 2, 3}
	stdout := permissionedCmd.Exec(t, "add-credit", "--target-pubkey", fmt.Sprintf("0x%x", targetPubkey), "-v", "99", "-k", "1")
	testutils.VerifyStdout(t, stdout, "Fee credit added successfully")

	require.Equal(t, 1, len(ss.SentTxs))
	for _, tx := range ss.SentTxs {
		require.EqualValues(t, 1, tx.NetworkID)
		require.EqualValues(t, 50, tx.PartitionID)
		require.Equal(t, permissioned.TransactionTypeSetFeeCredit, tx.Type)
		attr := permissioned.SetFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.Equal(t, uint64(9900000000), attr.Amount)
		require.EqualValues(t, templates.NewP2pkh256BytesFromKey(targetPubkey), attr.OwnerPredicate)
	}
}

func TestDeleteFeeCreditCmd(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	targetPubkey := []byte{1, 2, 3}
	as := mocksrv.NewAdminServiceMock(mocksrv.WithInfoResponse(
		&types.NodeInfoResponse{
			NetworkID:        1,
			PartitionID:      50,
			Name:             "tokens",
			PermissionedMode: true,
		}))
	ss := mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(hash.Sum256(targetPubkey),
			&sdktypes.Unit[any]{
				UnitID: tokens.NewFeeCreditRecordID(nil, []byte{1}),
				Data:   fc.FeeCreditRecord{Balance: 3, OwnerPredicate: nil},
			}))
	rpcUrl := mocksrv.StartServer(t, map[string]interface{}{
		"admin": as,
		"state": ss,
	})

	permissionedCmd := testutils.NewSubCmdExecutor(NewCmd, "--rpc-url", rpcUrl).WithHome(homedir)
	permissionedCmd.ExecWithError(t, "required flag(s)", "delete-credit")

	stdout := permissionedCmd.Exec(t, "delete-credit", "--target-pubkey", fmt.Sprintf("0x%x", targetPubkey), "-k", "1")
	testutils.VerifyStdout(t, stdout, "Fee credit deleted successfully")

	require.Equal(t, 1, len(ss.SentTxs))
	for _, tx := range ss.SentTxs {
		require.Equal(t, permissioned.TransactionTypeDeleteFeeCredit, tx.Type)
	}
}
