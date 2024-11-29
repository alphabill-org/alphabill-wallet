package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testmoney "github.com/alphabill-org/alphabill-wallet/internal/testutils/money"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
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
	tx, err := createTransferFC(types.NetworkLocal, money.DefaultPartitionID, maxFee, unitID, targetUnitID, 200, 0)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
	require.NotNil(t, tx.AuthProof)
}

func TestCreateAddFC(t *testing.T) {
	tx, err := createAddFC(types.NetworkLocal, money.DefaultPartitionID, unitID, ownerPredicate, &types.TxRecordProof{}, timeout, maxFee)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
	require.NotNil(t, tx.AuthProof)
}

func TestCreateTransferTx(t *testing.T) {
	tx, err := createTransferTx(types.NetworkLocal, money.DefaultPartitionID, pubKey, unitID, billValue, fcrID, timeout, counter)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
	require.NotNil(t, tx.AuthProof)
}

func TestExecBill_OK(t *testing.T) {
	rpcClientMock := testmoney.NewRpcClientMock()
	require.NoError(t, execInitialBill(context.Background(), rpcClientMock, types.NetworkLocal, money.DefaultPartitionID, unitID, fcrID, billValue, latestAdditionTime, ownerPredicate, counter))
}

func TestExecBill_NOK(t *testing.T) {
	rpcClientMock := testmoney.NewRpcClientMock(
		testmoney.WithError(errors.New("some error")),
	)
	require.ErrorContains(t, execInitialBill(context.Background(), rpcClientMock, types.NetworkLocal, money.DefaultPartitionID, unitID, fcrID, billValue, latestAdditionTime, ownerPredicate, counter), "some error")

}
