package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

var (
	pubKey             = test.RandomBytes(33)
	unitID             = test.RandomBytes(33)
	targetUnitID       = test.RandomBytes(33)
	fcrID              = test.RandomBytes(33)
	ownerPredicate     = test.RandomBytes(33)
	counter            = uint64(0)
	maxFee             = uint64(10)
	billValue          = uint64(100)
	timeout            = uint64(200)
	latestAdditionTime = uint64(300)
)

func TestCreateTransferFC(t *testing.T) {
	tx, err := createTransferFC(maxFee, unitID, targetUnitID, 200, 0)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
}

func TestCreateAddFC(t *testing.T) {
	tx, err := createAddFC(unitID, ownerPredicate, &types.TransactionRecord{}, &types.TxProof{}, timeout, maxFee)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
}

func TestCreateTransferTx(t *testing.T) {
	tx, err := createTransferTx(pubKey, unitID, billValue, fcrID, timeout, counter)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
}

func TestExecBill_OK(t *testing.T) {
	rpcClientMock := testutil.NewRpcClientMock()
	require.NoError(t, execInitialBill(context.Background(), rpcClientMock, unitID, fcrID, billValue, latestAdditionTime, ownerPredicate, counter))
}

func TestExecBill_NOK(t *testing.T) {
	rpcClientMock := testutil.NewRpcClientMock(
		testutil.WithError(errors.New("some error")),
	)
	require.ErrorContains(t, execInitialBill(context.Background(), rpcClientMock, unitID, fcrID, billValue, latestAdditionTime, ownerPredicate, counter), "some error")

}
