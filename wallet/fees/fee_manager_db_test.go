package fees

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

func TestDB_GetSetDeleteAddFeeCtx(t *testing.T) {
	s := createFeeManagerDB(t)
	accountID := []byte{4}

	// verify missing account returns nil and no error
	feeCtx, err := s.GetAddFeeContext(accountID)
	require.NoError(t, err)
	require.Nil(t, feeCtx)

	// store fee ctx
	feeCtx = &AddFeeCreditCtx{TargetAmount: 400}
	err = s.SetAddFeeContext(accountID, feeCtx)
	require.NoError(t, err)

	// verify stored equals actual
	storedFeeContext, err := s.GetAddFeeContext(accountID)
	require.NoError(t, err)
	require.Equal(t, feeCtx, storedFeeContext)

	// delete fee context
	err = s.DeleteAddFeeContext(accountID)
	require.NoError(t, err)

	// verify fee context is deleted
	feeCtx, err = s.GetAddFeeContext(accountID)
	require.NoError(t, err)
	require.Nil(t, feeCtx)
}

func TestDB_GetSetDeleteReclaimFeeCtx(t *testing.T) {
	s := createFeeManagerDB(t)
	accountID := []byte{4}
	partitionID := types.PartitionID(1)

	// verify missing account returns nil and no error
	feeCtx, err := s.GetReclaimFeeContext(accountID)
	require.NoError(t, err)
	require.Nil(t, feeCtx)

	// store fee ctx
	feeCtx = &ReclaimFeeCreditCtx{TargetPartitionID: partitionID}
	err = s.SetReclaimFeeContext(accountID, feeCtx)
	require.NoError(t, err)

	// verify stored equals actual
	storedFeeContext, err := s.GetReclaimFeeContext(accountID)
	require.NoError(t, err)
	require.Equal(t, feeCtx, storedFeeContext)

	// delete fee context
	err = s.DeleteReclaimFeeContext(accountID)
	require.NoError(t, err)

	// verify fee context is deleted
	feeCtx, err = s.GetReclaimFeeContext(accountID)
	require.NoError(t, err)
	require.Nil(t, feeCtx)
}
