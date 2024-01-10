package testutils

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
)

var (
	DefaultInitialBillID = money.NewBillID(nil, []byte{1})
	FCRID                = money.NewFeeCreditRecordID(nil, []byte{1})
)

func SpendInitialBillWithFeeCredits(t *testing.T, abNet *testpartition.AlphabillNetwork, initialBillValue uint64, pk []byte) uint64 {
	absoluteTimeout := uint64(10000)
	txFee := uint64(1)
	feeAmount := uint64(2)
	unitID := DefaultInitialBillID
	moneyPart, err := abNet.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)

	// create transferFC
	transferFC, err := createTransferFC(feeAmount+txFee, money.DefaultSystemIdentifier, unitID, FCRID, 0, absoluteTimeout)
	require.NoError(t, err)

	// send transferFC
	require.NoError(t, moneyPart.SubmitTx(transferFC))
	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, moneyPart, transferFC)
	require.NoError(t, err, "transfer fee credit tx failed")
	// verify proof
	require.NoError(t, types.VerifyTxProof(transferFCProof, transferFCRecord, abNet.RootPartition.TrustBase, crypto.SHA256))
	unitState, err := testpartition.WaitUnitProof(t, moneyPart, DefaultInitialBillID, transferFC)
	require.NoError(t, err)
	ucValidator, err := abNet.GetValidator(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	require.NoError(t, types.VerifyUnitStateProof(unitState.Proof, crypto.SHA256, unitState.UnitData, ucValidator))
	var bill money.BillData
	require.NoError(t, unitState.UnmarshalUnitData(&bill))
	require.EqualValues(t, initialBillValue-txFee-feeAmount, bill.V)
	// create addFC
	addFC, err := createAddFC(money.DefaultSystemIdentifier, FCRID, templates.AlwaysTrueBytes(), transferFCRecord, transferFCProof, absoluteTimeout, feeAmount)
	require.NoError(t, err)

	// send addFC
	require.NoError(t, moneyPart.SubmitTx(addFC))
	_, _, err = testpartition.WaitTxProof(t, moneyPart, addFC)
	require.NoError(t, err, "add fee credit tx failed")

	// create transfer tx
	remainingValue := initialBillValue - feeAmount - txFee
	tx, err := createTransferTx(pk, unitID, remainingValue, FCRID, absoluteTimeout, transferFCRecord.TransactionOrder.Hash(crypto.SHA256))
	require.NoError(t, err)

	// send transfer tx
	require.NoError(t, moneyPart.SubmitTx(tx))
	_, _, err = testpartition.WaitTxProof(t, moneyPart, tx)
	require.NoError(t, err, "transfer tx failed")
	return remainingValue
}

func createTransferTx(pubKey []byte, billID []byte, billValue uint64, fcrID []byte, timeout uint64, backlink []byte) (*types.TransactionOrder, error) {
	attr := &money.TransferAttributes{
		NewBearer:   templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(pubKey)),
		TargetValue: billValue,
		Backlink:    backlink,
	}
	attrBytes, err := cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			UnitID:     billID,
			Type:       money.PayloadTypeTransfer,
			SystemID:   money.DefaultSystemIdentifier,
			Attributes: attrBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: 1,
				FeeCreditRecordID: fcrID,
			},
		},
		OwnerProof: nil,
	}
	return tx, nil
}

func createTransferFC(feeAmount uint64, targetSystemID types.SystemID, unitID, targetUnitID []byte, t1, t2 uint64) (*types.TransactionOrder, error) {
	attr := &transactions.TransferFeeCreditAttributes{
		Amount:                 feeAmount,
		TargetSystemIdentifier: targetSystemID,
		TargetRecordID:         targetUnitID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
	}
	attrBytes, err := cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   money.DefaultSystemIdentifier,
			Type:       transactions.PayloadTypeTransferFeeCredit,
			UnitID:     unitID,
			Attributes: attrBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           t2,
				MaxTransactionFee: 1,
			},
		},
		OwnerProof: nil,
	}
	return tx, nil
}

func createAddFC(systemID types.SystemID, unitID []byte, ownerCondition []byte, transferFC *types.TransactionRecord, transferFCProof *types.TxProof, timeout uint64, maxFee uint64) (*types.TransactionOrder, error) {
	attr := &transactions.AddFeeCreditAttributes{
		FeeCreditTransfer:       transferFC,
		FeeCreditTransferProof:  transferFCProof,
		FeeCreditOwnerCondition: ownerCondition,
	}
	attrBytes, err := cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   systemID,
			Type:       transactions.PayloadTypeAddFeeCredit,
			UnitID:     unitID,
			Attributes: attrBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: maxFee,
			},
		},
		OwnerProof: nil,
	}
	return tx, nil
}
