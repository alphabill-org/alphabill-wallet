package permissioned

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	tokenid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestAddFeeCreditCmd(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	as := mocksrv.NewAdminServiceMock(mocksrv.WithInfoResponse(
		&sdktypes.NodeInfoResponse{
			NetworkID:        1,
			PartitionID:      50,
			PartitionTypeID:  tokens.PartitionTypeID,
			PermissionedMode: true,
		}))
	ss := mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t),
			&sdktypes.Unit[any]{
				UnitID: tokenid.NewFeeCreditRecordID(t),
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
		&sdktypes.NodeInfoResponse{
			NetworkID:        1,
			PartitionID:      50,
			PartitionTypeID:  tokens.PartitionTypeID,
			PermissionedMode: true,
		}))
	ss := mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(hash.Sum256(targetPubkey),
			&sdktypes.Unit[any]{
				UnitID: tokenid.NewFeeCreditRecordID(t),
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

func TestListFeeCreditCmd(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	as := mocksrv.NewAdminServiceMock(mocksrv.WithInfoResponse(
		&sdktypes.NodeInfoResponse{
			NetworkID:        1,
			PartitionID:      50,
			PartitionTypeID:  tokens.PartitionTypeID,
			PermissionedMode: true,
		}))
	unitIDs := []string{
		"0x16D9C685A84B761A6F96FA81BC59AEB55A697BF64F029F8A2204C6EC3622AC8A10",
		"0x2EDD94BD4AD931F281EBE9CD6BCF0180F4AE0B607370C8D63FCF72D81DB6C8E710",
	}
	ss := mocksrv.NewStateServiceMock(
		mocksrv.WithUnits(
			&sdktypes.Unit[any]{
				UnitID: decodeHex(t, unitIDs[0]),
				Data:   fc.FeeCreditRecord{Balance: 1},
			},
			&sdktypes.Unit[any]{
				UnitID: decodeHex(t, unitIDs[1]),
				Data:   fc.FeeCreditRecord{Balance: 2},
			},
		))
	rpcUrl := mocksrv.StartServer(t, map[string]interface{}{
		"admin": as,
		"state": ss,
	})

	permissionedCmd := testutils.NewSubCmdExecutor(NewCmd, "--rpc-url", rpcUrl).WithHome(homedir)

	// test default list-credit command
	stdout := permissionedCmd.Exec(t, "list-credit")
	require.Len(t, stdout.Lines, 3)
	require.Equal(t, "Total Fee Credit Records: 2", stdout.Lines[0])
	require.Equal(t, unitIDs[0], stdout.Lines[1])
	require.Equal(t, unitIDs[1], stdout.Lines[2])

	// test verbose list-credit command
	stdout = permissionedCmd.Exec(t, "list-credit", "--verbose")
	require.Len(t, stdout.Lines, 3)
	require.Equal(t, "Total Fee Credit Records: 2", stdout.Lines[0])
	require.Equal(t, `{"networkId":0,"partitionId":0,"id":"0x16d9c685a84b761a6f96fa81bc59aeb55a697bf64f029f8a2204c6ec3622ac8a10","balance":1,"ownerPredicate":"","minLifetime":0,"lockStatus":0,"counter":0}`, stdout.Lines[1])
	require.Equal(t, `{"networkId":0,"partitionId":0,"id":"0x2edd94bd4ad931f281ebe9cd6bcf0180f4ae0b607370c8d63fcf72d81db6c8e710","balance":2,"ownerPredicate":"","minLifetime":0,"lockStatus":0,"counter":0}`, stdout.Lines[2])
}

func decodeHex(t *testing.T, s string) []byte {
	b, err := hexutil.Decode(s)
	require.NoError(t, err)
	return b
}
