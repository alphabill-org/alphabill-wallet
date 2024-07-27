package txbuilder

import (
	"fmt"
	"slices"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

const MaxFee = uint64(10)

// CreateTransactions creates 1 to N P2PKH transactions from given bills until target amount is reached.
// If there exists a bill with value equal to the given amount then transfer transaction is created using that bill,
// otherwise bills are selected in the given order.
func CreateTransactions(pubKey []byte, amount uint64, systemID types.SystemID, bills []sdktypes.Bill, k *account.AccountKey, timeout uint64, fcrID, refNo []byte) ([]*types.TransactionOrder, error) {
	billIndex := slices.IndexFunc(bills, func(b sdktypes.Bill) bool { return b.Value() == amount })
	if billIndex >= 0 {
		ownerPredicate := templates.NewP2pkh256BytesFromKey(pubKey)
		tx, err := bills[billIndex].Transfer(ownerPredicate,
			sdktypes.WithTimeout(timeout),
			sdktypes.WithFeeCreditRecordID(fcrID),
			sdktypes.WithReferenceNumber(refNo),
			sdktypes.WithOwnerProof(sdktypes.NewP2pkhProofGenerator(k.PrivKey, k.PubKey)))
		if err != nil {
			return nil, err
		}
		return []*types.TransactionOrder{tx}, nil
	}
	var txs []*types.TransactionOrder
	var accumulatedSum uint64
	for _, b := range bills {
		remainingAmount := amount - accumulatedSum
		tx, err := createTransaction(pubKey, k, remainingAmount, b, timeout, fcrID, refNo)
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
		accumulatedSum += b.Value()
		if accumulatedSum >= amount {
			return txs, nil
		}
	}
	return nil, fmt.Errorf("insufficient balance for transaction, trying to send %d have %d", amount, accumulatedSum)
}

// createTransaction creates a P2PKH transfer or split transaction using the given bill.
func createTransaction(receiverPubKey []byte, k *account.AccountKey, amount uint64, bill sdktypes.Bill, timeout uint64, fcrID, refNo []byte) (*types.TransactionOrder, error) {
	if bill.Value() <= amount {
		ownerPredicate := templates.NewP2pkh256BytesFromKey(receiverPubKey)
		return bill.Transfer(ownerPredicate,
			sdktypes.WithTimeout(timeout),
			sdktypes.WithFeeCreditRecordID(fcrID),
			sdktypes.WithReferenceNumber(refNo),
			sdktypes.WithOwnerProof(sdktypes.NewP2pkhProofGenerator(k.PrivKey, k.PubKey)))
	}
	targetUnits := []*money.TargetUnit{
		{Amount: amount, OwnerCondition: templates.NewP2pkh256BytesFromKey(receiverPubKey)},
	}
	return bill.Split(targetUnits,
		sdktypes.WithTimeout(timeout),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithReferenceNumber(refNo),
		sdktypes.WithOwnerProof(sdktypes.NewP2pkhProofGenerator(k.PrivKey, k.PubKey)))
}
