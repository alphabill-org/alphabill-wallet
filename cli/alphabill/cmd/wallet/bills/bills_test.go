package bills

import (
	"testing"

	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	basetypes "github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	"github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/stretchr/testify/require"
)

func TestWalletBillsListCmd_EmptyWallet(t *testing.T) {
	pdr := moneyid.PDR()
	rpcUrl := mocksrv.StartStateApiServer(t, &pdr, mocksrv.NewStateServiceMock())
	homedir := testutils.CreateNewTestWallet(t)
	billsCmd := testutils.NewSubCmdExecutor(NewBillsCmd, "--rpc-url", rpcUrl).WithHome(homedir)

	testutils.VerifyStdout(t, billsCmd.Exec(t, "list"), "Account #1 - empty")
}

func TestWalletBillsListCmd_Single(t *testing.T) {
	pdr := moneyid.PDR()
	rpcUrl := mocksrv.StartStateApiServer(t, &pdr, mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit(
		testutils.TestPubKey0Hash(t),
		&types.Unit[any]{
			UnitID: moneyid.BillIDWithSuffix(t, 1, &pdr),
			Data:   money.BillData{Value: 1e8},
		},
	)))
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	billsCmd := testutils.NewSubCmdExecutor(NewBillsCmd, "--rpc-url", rpcUrl).WithHome(homedir)

	testutils.VerifyStdout(t, billsCmd.Exec(t, "list"), "#1 0x000000000000000000000000000000000000000000000000000000000000000101 1.000'000'00")
}

func TestWalletBillsListCmd_Multiple(t *testing.T) {
	pdr := moneyid.PDR()
	rpcUrl := mocksrv.StartStateApiServer(t, &pdr, mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t), &types.Unit[any]{UnitID: moneyid.BillIDWithSuffix(t, 1, &pdr), Data: money.BillData{Value: 1}}),
		mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t), &types.Unit[any]{UnitID: moneyid.BillIDWithSuffix(t, 2, &pdr), Data: money.BillData{Value: 2}}),
		mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t), &types.Unit[any]{UnitID: moneyid.BillIDWithSuffix(t, 3, &pdr), Data: money.BillData{Value: 3}}),
		mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t), &types.Unit[any]{UnitID: moneyid.BillIDWithSuffix(t, 4, &pdr), Data: money.BillData{Value: 4}}),
	))
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	billsCmd := testutils.NewSubCmdExecutor(NewBillsCmd, "--rpc-url", rpcUrl).WithHome(homedir)

	stdout := billsCmd.Exec(t, "list")
	require.Len(t, stdout.Lines, 5)
	require.Equal(t, stdout.Lines[0], "Account #1")
	require.Equal(t, "#1 0x000000000000000000000000000000000000000000000000000000000000000101 0.000'000'01", stdout.Lines[1])
	require.Equal(t, "#2 0x000000000000000000000000000000000000000000000000000000000000000201 0.000'000'02", stdout.Lines[2])
	require.Equal(t, "#3 0x000000000000000000000000000000000000000000000000000000000000000301 0.000'000'03", stdout.Lines[3])
	require.Equal(t, "#4 0x000000000000000000000000000000000000000000000000000000000000000401 0.000'000'04", stdout.Lines[4])
}

func TestWalletBillsListCmd_ExtraAccount(t *testing.T) {
	pdr := moneyid.PDR()
	rpcUrl := mocksrv.StartStateApiServer(t, &pdr, mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(testutils.TestPubKey1Hash(t), &types.Unit[any]{UnitID: moneyid.BillIDWithSuffix(t, 1, &pdr), Data: money.BillData{Value: 1}}),
	))
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic(), testutils.WithNumberOfAccounts(2))
	billsCmd := testutils.NewSubCmdExecutor(NewBillsCmd, "--rpc-url", rpcUrl).WithHome(homedir)

	// verify list bills for specific account only shows given account bills
	stdout := billsCmd.Exec(t, "list", "-k", "2")
	lines := stdout.Lines
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "Account #2")
	require.Contains(t, lines[1], "#1")
}

func TestWalletBillsListCmd_ExtraAccountTotal(t *testing.T) {
	pdr := moneyid.PDR()
	rpcUrl := mocksrv.StartStateApiServer(t, &pdr, mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t), &types.Unit[any]{UnitID: moneyid.BillIDWithSuffix(t, 1, &pdr), Data: money.BillData{Value: 1e9}}),
	))
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic(), testutils.WithNumberOfAccounts(2))
	billsCmd := testutils.NewSubCmdExecutor(NewBillsCmd, "--rpc-url", rpcUrl).WithHome(homedir)

	// verify both accounts are listed
	stdout := billsCmd.Exec(t, "list")
	testutils.VerifyStdout(t, stdout, "Account #1")
	testutils.VerifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000101 10")
	testutils.VerifyStdout(t, stdout, "Account #2 - empty")
}

func TestWalletBillsListCmd_ShowLockedBills(t *testing.T) {
	pdr := moneyid.PDR()
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, &pdr, mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t), &types.Unit[any]{UnitID: moneyid.BillIDWithSuffix(t, 1, &pdr), Data: money.BillData{Value: 1e8}, StateLockTx: []byte{1}}),
		mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t), &types.Unit[any]{UnitID: moneyid.BillIDWithSuffix(t, 2, &pdr), Data: money.BillData{Value: 1e8}, StateLockTx: []byte{2}}),
		mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t), &types.Unit[any]{UnitID: moneyid.BillIDWithSuffix(t, 3, &pdr), Data: money.BillData{Value: 1e8}, StateLockTx: []byte{15}}),
	))
	billsCmd := testutils.NewSubCmdExecutor(NewBillsCmd, "--rpc-url", rpcUrl).WithHome(homedir)

	stdout := billsCmd.Exec(t, "list")
	require.Len(t, stdout.Lines, 4)
	require.Equal(t, "#1 0x000000000000000000000000000000000000000000000000000000000000000101 1.000'000'00 locked='0x01'", stdout.Lines[1])
	require.Equal(t, "#2 0x000000000000000000000000000000000000000000000000000000000000000201 1.000'000'00 locked='0x02'", stdout.Lines[2])
	require.Equal(t, "#3 0x000000000000000000000000000000000000000000000000000000000000000301 1.000'000'00 locked='0x0F'", stdout.Lines[3])
}

func TestWalletBillsLockUnlockCmd_Nok(t *testing.T) {
	pdr := moneyid.PDR()
	billID := moneyid.BillIDWithSuffix(t, 1, &pdr)
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic(), testutils.WithNumberOfAccounts(2))

	// create FeeCredit unit ID similar to the bill ID (type part differs)
	fcrID, err := pdr.ComposeUnitID(basetypes.ShardID{}, money.FeeCreditRecordUnitType, func(b []byte) error { b[len(b)-1] = 1; return nil })
	require.NoError(t, err)
	rpcUrl := mocksrv.StartServer(t, map[string]interface{}{
		"state": mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit(testutils.TestPubKey0Hash(t),
			&types.Unit[any]{
				UnitID: fcrID,
				Data:   fc.FeeCreditRecord{Balance: 100},
			})),
		"admin": mocksrv.NewAdminServiceMock(),
	})

	billsCmd := testutils.NewSubCmdExecutor(NewBillsCmd, "--rpc-url", rpcUrl).WithHome(homedir)

	billsCmd.ExecWithError(t, "bill not found", "lock", "--bill-id", billID.String())
	billsCmd.ExecWithError(t, "bill not found", "unlock", "--bill-id", billID.String())

	billsCmd.ExecWithError(t, "not enough fee credit in wallet", "lock", "--key", "2", "--bill-id", billID.String())
	billsCmd.ExecWithError(t, "not enough fee credit in wallet", "unlock", "--key", "2", "--bill-id", billID.String())
}
