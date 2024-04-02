package tx_builder

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
)

const MaxFee = uint64(1)

// CreateTransactions creates 1 to N P2PKH transactions from given bills until target amount is reached.
// If there exists a bill with value equal to the given amount then transfer transaction is created using that bill,
// otherwise bills are selected in the given order.
func CreateTransactions(pubKey []byte, amount uint64, systemID types.SystemID, bills []*api.Bill, k *account.AccountKey, timeout uint64, fcrID []byte) ([]*types.TransactionOrder, error) {
	billIndex := slices.IndexFunc(bills, func(b *api.Bill) bool { return b.Value() == amount })
	if billIndex >= 0 {
		tx, err := NewTransferTx(pubKey, k, systemID, bills[billIndex], timeout, fcrID)
		if err != nil {
			return nil, err
		}
		return []*types.TransactionOrder{tx}, nil
	}
	var txs []*types.TransactionOrder
	var accumulatedSum uint64
	for _, b := range bills {
		remainingAmount := amount - accumulatedSum
		tx, err := CreateTransaction(pubKey, k, remainingAmount, systemID, b, timeout, fcrID)
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

// CreateTransaction creates a P2PKH transfer or split transaction using the given bill.
func CreateTransaction(receiverPubKey []byte, k *account.AccountKey, amount uint64, systemID types.SystemID, bill *api.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	if bill.Value() <= amount {
		return NewTransferTx(receiverPubKey, k, systemID, bill, timeout, fcrID)
	}
	targetUnits := []*money.TargetUnit{
		{Amount: amount, OwnerCondition: templates.NewP2pkh256BytesFromKey(receiverPubKey)},
	}
	remainingValue := bill.Value() - amount
	return NewSplitTx(targetUnits, remainingValue, k, systemID, bill, timeout, fcrID)
}

// NewTransferTx creates a P2PKH transfer transaction.
func NewTransferTx(receiverPubKey []byte, k *account.AccountKey, systemID types.SystemID, bill *api.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	attr := &money.TransferAttributes{
		NewBearer:   templates.NewP2pkh256BytesFromKey(receiverPubKey),
		TargetValue: bill.Value(),
		Counter:     bill.Counter(),
	}
	txPayload, err := NewTxPayload(systemID, money.PayloadTypeTransfer, bill.ID, timeout, fcrID, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, k)
}

// NewSplitTx creates a P2PKH split transaction.
func NewSplitTx(targetUnits []*money.TargetUnit, remainingValue uint64, k *account.AccountKey, systemID types.SystemID, bill *api.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	attr := &money.SplitAttributes{
		TargetUnits:    targetUnits,
		RemainingValue: remainingValue,
		Counter:        bill.Counter(),
	}
	txPayload, err := NewTxPayload(systemID, money.PayloadTypeSplit, bill.ID, timeout, fcrID, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, k)
}

func NewDustTx(ac *account.AccountKey, systemID types.SystemID, bill *api.Bill, targetBillID []byte, targetUnitCounter, timeout uint64) (*types.TransactionOrder, error) {
	attr := &money.TransferDCAttributes{
		TargetUnitID:      targetBillID,
		TargetUnitCounter: targetUnitCounter,
		Value:             bill.Value(),
		Counter:           bill.Counter(),
	}
	txPayload, err := NewTxPayload(systemID, money.PayloadTypeTransDC, bill.ID, timeout, money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256), attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewSwapTx(k *account.AccountKey, systemID types.SystemID, dcProofs []*wallet.Proof, targetUnitID []byte, timeout uint64) (*types.TransactionOrder, error) {
	if len(dcProofs) == 0 {
		return nil, errors.New("cannot create swap transaction as no dust transfer proofs exist")
	}
	// sort proofs by ids smallest first
	sort.Slice(dcProofs, func(i, j int) bool {
		return bytes.Compare(dcProofs[i].TxRecord.TransactionOrder.UnitID(), dcProofs[j].TxRecord.TransactionOrder.UnitID()) < 0
	})
	var dustTransferProofs []*types.TxProof
	var dustTransferRecords []*types.TransactionRecord
	var billValueSum uint64
	for _, p := range dcProofs {
		dustTransferRecords = append(dustTransferRecords, p.TxRecord)
		dustTransferProofs = append(dustTransferProofs, p.TxProof)
		var attr *money.TransferDCAttributes
		if err := p.TxRecord.TransactionOrder.UnmarshalAttributes(&attr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal dust transfer tx: %w", err)
		}
		billValueSum += attr.Value
	}
	attr := &money.SwapDCAttributes{
		OwnerCondition:   templates.NewP2pkh256BytesFromKeyHash(k.PubKeyHash.Sha256),
		DcTransfers:      dustTransferRecords,
		DcTransferProofs: dustTransferProofs,
		TargetValue:      billValueSum,
	}
	swapTx, err := NewTxPayload(systemID, money.PayloadTypeSwapDC, targetUnitID, timeout, money.NewFeeCreditRecordID(nil, k.PubKeyHash.Sha256), attr)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap transaction: %w", err)
	}
	payload, err := SignPayload(swapTx, k)
	if err != nil {
		return nil, fmt.Errorf("failed to sign swap transaction: %w", err)
	}
	return payload, nil
}

func NewLockTx(ac *account.AccountKey, systemID types.SystemID, unitID []byte, counter uint64, lockStatus, timeout uint64) (*types.TransactionOrder, error) {
	attr := &money.LockAttributes{
		LockStatus: lockStatus,
		Counter:    counter,
	}
	txPayload, err := NewTxPayload(systemID, money.PayloadTypeLock, unitID, timeout, money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256), attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewUnlockTx(ac *account.AccountKey, systemID types.SystemID, b *api.Bill, timeout uint64) (*types.TransactionOrder, error) {
	attr := &money.UnlockAttributes{
		Counter: b.Counter(),
	}
	txPayload, err := NewTxPayload(systemID, money.PayloadTypeUnlock, b.ID, timeout, money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256), attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewLockFCTx(ac *account.AccountKey, systemID types.SystemID, fcb *api.FeeCreditBill, lockStatus uint64, timeout uint64) (*types.TransactionOrder, error) {
	attr := &transactions.LockFeeCreditAttributes{
		LockStatus: lockStatus,
		Backlink:   fcb.Backlink(),
	}
	txPayload, err := NewTxPayload(systemID, transactions.PayloadTypeLockFeeCredit, fcb.ID, timeout, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewUnlockFCTx(ac *account.AccountKey, systemID types.SystemID, fcb *api.FeeCreditBill, timeout uint64) (*types.TransactionOrder, error) {
	attr := &transactions.UnlockFeeCreditAttributes{
		Backlink: fcb.Backlink(),
	}
	txPayload, err := NewTxPayload(systemID, transactions.PayloadTypeUnlockFeeCredit, fcb.ID, timeout, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewTransferFCTx(amount uint64, targetRecordID []byte, targetUnitBacklink []byte, k *account.AccountKey, moneySystemID, targetSystemID types.SystemID, unitID []byte, counter, timeout, t1, t2 uint64) (*types.TransactionOrder, error) {
	attr := &transactions.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: targetSystemID,
		TargetRecordID:         targetRecordID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
		TargetUnitBacklink:     targetUnitBacklink,
		Counter:                counter,
	}
	txPayload, err := NewTxPayload(moneySystemID, transactions.PayloadTypeTransferFeeCredit, unitID, timeout, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, k)
}

func NewAddFCTx(unitID []byte, fcProof *wallet.Proof, ac *account.AccountKey, systemID types.SystemID, timeout uint64) (*types.TransactionOrder, error) {
	attr := &transactions.AddFeeCreditAttributes{
		FeeCreditOwnerCondition: templates.NewP2pkh256BytesFromKeyHash(ac.PubKeyHash.Sha256),
		FeeCreditTransfer:       fcProof.TxRecord,
		FeeCreditTransferProof:  fcProof.TxProof,
	}
	txPayload, err := NewTxPayload(systemID, transactions.PayloadTypeAddFeeCredit, unitID, timeout, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewCloseFCTx(systemID types.SystemID, unitID []byte, timeout uint64, amount uint64, targetUnitID []byte, targetUnitBacklink uint64, k *account.AccountKey) (*types.TransactionOrder, error) {
	attr := &transactions.CloseFeeCreditAttributes{
		Amount:            amount,
		TargetUnitID:      targetUnitID,
		TargetUnitCounter: targetUnitBacklink,
	}
	txPayload, err := NewTxPayload(systemID, transactions.PayloadTypeCloseFeeCredit, unitID, timeout, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, k)
}

func NewReclaimFCTx(systemID types.SystemID, unitID []byte, timeout uint64, fcProof *wallet.Proof, counter uint64, k *account.AccountKey) (*types.TransactionOrder, error) {
	attr := &transactions.ReclaimFeeCreditAttributes{
		CloseFeeCreditTransfer: fcProof.TxRecord,
		CloseFeeCreditProof:    fcProof.TxProof,
		Counter:                counter,
	}
	txPayload, err := NewTxPayload(systemID, transactions.PayloadTypeReclaimFeeCredit, unitID, timeout, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, k)
}

func NewTxPayload(systemID types.SystemID, txType string, unitID types.UnitID, timeout uint64, fcrID []byte, attr interface{}) (*types.Payload, error) {
	attrBytes, err := types.Cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	return &types.Payload{
		SystemID:   systemID,
		Type:       txType,
		UnitID:     unitID,
		Attributes: attrBytes,
		ClientMetadata: &types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: MaxFee,
			FeeCreditRecordID: fcrID,
		},
	}, nil
}

func SignPayload(payload *types.Payload, ac *account.AccountKey) (*types.TransactionOrder, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := payload.Bytes()
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(payloadBytes)
	if err != nil {
		return nil, err
	}
	return &types.TransactionOrder{
		Payload:    payload,
		OwnerProof: templates.NewP2pkh256SignatureBytes(sig, ac.PubKey),
	}, nil
}
