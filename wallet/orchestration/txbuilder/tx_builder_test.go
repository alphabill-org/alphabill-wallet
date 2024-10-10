package txbuilder

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

const testMnemonic = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"

var (
	accountKey, _ = account.NewKeys(testMnemonic)
)

func TestNewAddVarTx_OK(t *testing.T) {
	timeout := uint64(10)
	networkID := types.NetworkLocal
	systemID := orchestration.DefaultSystemID
	unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
	_var := orchestration.ValidatorAssignmentRecord{}

	tx, err := NewAddVarTx(_var, networkID, systemID, unitID, timeout, 3, accountKey.AccountKey)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, systemID, tx.GetSystemID())
	require.EqualValues(t, unitID, tx.GetUnitID())
	require.EqualValues(t, timeout, tx.Timeout())
	require.EqualValues(t, 3, tx.MaxFee())
	require.Nil(t, tx.FeeCreditRecordID())
	require.Nil(t, tx.FeeProof)
	require.NotNil(t, tx.AuthProof)

	require.EqualValues(t, orchestration.TransactionTypeAddVAR, tx.Type)
	var attr *orchestration.AddVarAttributes
	err = tx.UnmarshalAttributes(&attr)
	require.NoError(t, err)
	require.Equal(t, _var, attr.Var)
}

func TestNewAddVarTxUnsigned_OK(t *testing.T) {
	timeout := uint64(10)
	networkID := types.NetworkLocal
	systemID := orchestration.DefaultSystemID
	unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
	_var := orchestration.ValidatorAssignmentRecord{}

	tx, err := NewAddVarTx(_var, networkID, systemID, unitID, timeout, 5, nil)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, systemID, tx.GetSystemID())
	require.EqualValues(t, unitID, tx.GetUnitID())
	require.EqualValues(t, timeout, tx.Timeout())
	require.EqualValues(t, 5, tx.MaxFee())
	require.Nil(t, tx.FeeCreditRecordID())
	require.Nil(t, tx.FeeProof)
	require.Nil(t, tx.AuthProof)

	require.EqualValues(t, orchestration.TransactionTypeAddVAR, tx.Type)
	var attr *orchestration.AddVarAttributes
	err = tx.UnmarshalAttributes(&attr)
	require.NoError(t, err)
	require.Equal(t, _var, attr.Var)
}
