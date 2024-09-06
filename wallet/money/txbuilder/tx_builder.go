package txbuilder

import (
	"fmt"
	"slices"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

// CreateTransactions creates 1 to N P2PKH transactions from given bills until target amount is reached.
// If there exists a bill with value equal to the given amount then transfer transaction is created using that bill,
// otherwise bills are selected in the given order.
func CreateTransactions(pubKey []byte, amount uint64, bills []*sdktypes.Bill, txSigner *sdktypes.MoneyTxSigner, timeout uint64, fcrID, refNo []byte, maxFee uint64) ([]*types.TransactionOrder, error) {
	billIndex := slices.IndexFunc(bills, func(b *sdktypes.Bill) bool { return b.Value == amount })
	if billIndex >= 0 {
		ownerPredicate := templates.NewP2pkh256BytesFromKey(pubKey)
		txo, err := bills[billIndex].Transfer(ownerPredicate,
			sdktypes.WithTimeout(timeout),
			sdktypes.WithFeeCreditRecordID(fcrID),
			sdktypes.WithMaxFee(maxFee),
			sdktypes.WithReferenceNumber(refNo),
		)
		if err != nil {
			return nil, err
		}
		if err = txSigner.SignTx(txo); err != nil {
			return nil, fmt.Errorf("failed to sign tx: %w", err)
		}
		return []*types.TransactionOrder{txo}, nil
	}
	var txs []*types.TransactionOrder
	var accumulatedSum uint64
	for _, b := range bills {
		remainingAmount := amount - accumulatedSum
		tx, err := createTransaction(pubKey, txSigner, remainingAmount, b, timeout, fcrID, refNo, maxFee)
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
		accumulatedSum += b.Value
		if accumulatedSum >= amount {
			return txs, nil
		}
	}
	return nil, fmt.Errorf("insufficient balance for transaction, trying to send %d have %d", amount, accumulatedSum)
}

// createTransaction creates a P2PKH transfer or split transaction using the given bill.
func createTransaction(receiverPubKey []byte, txSigner *sdktypes.MoneyTxSigner, amount uint64, bill *sdktypes.Bill, timeout uint64, fcrID, refNo []byte, maxFee uint64) (*types.TransactionOrder, error) {
	if bill.Value <= amount {
		ownerPredicate := templates.NewP2pkh256BytesFromKey(receiverPubKey)
		txo, err := bill.Transfer(ownerPredicate,
			sdktypes.WithTimeout(timeout),
			sdktypes.WithFeeCreditRecordID(fcrID),
			sdktypes.WithMaxFee(maxFee),
			sdktypes.WithReferenceNumber(refNo),
		)
		if err != nil {
			return nil, err
		}
		if err = txSigner.SignTx(txo); err != nil {
			return nil, fmt.Errorf("failed to sign tx: %w", err)
		}
		return txo, nil
	}
	targetUnits := []*money.TargetUnit{
		{
			Amount:         amount,
			OwnerPredicate: templates.NewP2pkh256BytesFromKey(receiverPubKey),
		},
	}
	txo, err := bill.Split(targetUnits,
		sdktypes.WithTimeout(timeout),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(maxFee),
		sdktypes.WithReferenceNumber(refNo),
	)
	if err != nil {
		return nil, err
	}
	if err = txSigner.SignTx(txo); err != nil {
		return nil, fmt.Errorf("failed to sign tx: %w", err)
	}
	return txo, nil
}
