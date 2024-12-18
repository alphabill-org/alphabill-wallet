package txbuilder

import (
	"testing"

	"github.com/stretchr/testify/require"

	orchid "github.com/alphabill-org/alphabill-go-base/testutils/orchestration"
	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

const testMnemonic = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"

var (
	accountKey, _ = account.NewKeys(testMnemonic)
)

func TestNewAddVarTx_OK(t *testing.T) {
	timeout := uint64(10)
	networkID := types.NetworkLocal
	partitionID := orchestration.DefaultPartitionID
	unitID := orchid.NewVarID(t)
	_var := orchestration.ValidatorAssignmentRecord{}

	tx, err := NewAddVarTx(_var, networkID, partitionID, unitID, timeout, 3, accountKey.AccountKey)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, partitionID, tx.GetPartitionID())
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
	partitionID := orchestration.DefaultPartitionID
	unitID := orchid.NewVarID(t)
	_var := orchestration.ValidatorAssignmentRecord{}

	tx, err := NewAddVarTx(_var, networkID, partitionID, unitID, timeout, 5, nil)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, partitionID, tx.GetPartitionID())
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
