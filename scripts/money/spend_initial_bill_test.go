package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
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
	tx, err := createTransferFC(types.NetworkLocal, money.DefaultSystemID, maxFee, unitID, targetUnitID, 200, 0)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
	require.NotNil(t, tx.AuthProof)
}

func TestCreateAddFC(t *testing.T) {
	tx, err := createAddFC(types.NetworkLocal, money.DefaultSystemID, unitID, ownerPredicate, &types.TxRecordProof{}, timeout, maxFee)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
	require.NotNil(t, tx.AuthProof)
}

func TestCreateTransferTx(t *testing.T) {
	tx, err := createTransferTx(types.NetworkLocal, money.DefaultSystemID, pubKey, unitID, billValue, fcrID, timeout, counter)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
	require.NotNil(t, tx.AuthProof)
}

func TestExecBill_OK(t *testing.T) {
	rpcClientMock := testutil.NewRpcClientMock()
	require.NoError(t, execInitialBill(context.Background(), rpcClientMock, types.NetworkLocal, money.DefaultSystemID, unitID, fcrID, billValue, latestAdditionTime, ownerPredicate, counter))
}

func TestExecBill_NOK(t *testing.T) {
	rpcClientMock := testutil.NewRpcClientMock(
		testutil.WithError(errors.New("some error")),
	)
	require.ErrorContains(t, execInitialBill(context.Background(), rpcClientMock, types.NetworkLocal, money.DefaultSystemID, unitID, fcrID, billValue, latestAdditionTime, ownerPredicate, counter), "some error")

}
