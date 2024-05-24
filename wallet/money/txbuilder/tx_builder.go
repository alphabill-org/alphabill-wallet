package txbuilder

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
	mwtypes "github.com/alphabill-org/alphabill-wallet/wallet/money/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txbuilder"
)

const MaxFee = uint64(1)

// CreateTransactions creates 1 to N P2PKH transactions from given bills until target amount is reached.
// If there exists a bill with value equal to the given amount then transfer transaction is created using that bill,
// otherwise bills are selected in the given order.
func CreateTransactions(pubKey []byte, amount uint64, systemID types.SystemID, bills []*api.Bill, k *account.AccountKey, timeout uint64, fcrID, refNo []byte) ([]*types.TransactionOrder, error) {
	billIndex := slices.IndexFunc(bills, func(b *api.Bill) bool { return b.Value() == amount })
	if billIndex >= 0 {
		tx, err := NewTransferTx(pubKey, k, systemID, bills[billIndex], timeout, fcrID, refNo)
		if err != nil {
			return nil, err
		}
		return []*types.TransactionOrder{tx}, nil
	}
	var txs []*types.TransactionOrder
	var accumulatedSum uint64
	for _, b := range bills {
		remainingAmount := amount - accumulatedSum
		tx, err := CreateTransaction(pubKey, k, remainingAmount, systemID, b, timeout, fcrID, refNo)
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
func CreateTransaction(receiverPubKey []byte, k *account.AccountKey, amount uint64, systemID types.SystemID, bill *api.Bill, timeout uint64, fcrID, refNo []byte) (*types.TransactionOrder, error) {
	if bill.Value() <= amount {
		return NewTransferTx(receiverPubKey, k, systemID, bill, timeout, fcrID, refNo)
	}
	targetUnits := []*money.TargetUnit{
		{Amount: amount, OwnerCondition: templates.NewP2pkh256BytesFromKey(receiverPubKey)},
	}
	remainingValue := bill.Value() - amount
	return NewSplitTx(targetUnits, remainingValue, k, systemID, bill, timeout, fcrID, refNo)
}

// NewTransferTx creates a P2PKH transfer transaction.
func NewTransferTx(receiverPubKey []byte, k *account.AccountKey, systemID types.SystemID, bill *api.Bill, timeout uint64, fcrID, refNo []byte) (*types.TransactionOrder, error) {
	attr := &money.TransferAttributes{
		NewBearer:   templates.NewP2pkh256BytesFromKey(receiverPubKey),
		TargetValue: bill.Value(),
		Counter:     bill.Counter(),
	}
	txPayload, err := txbuilder.NewTxPayload(systemID, money.PayloadTypeTransfer, bill.ID, timeout, fcrID, refNo, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, k)
}

// NewSplitTx creates a P2PKH split transaction.
func NewSplitTx(targetUnits []*money.TargetUnit, remainingValue uint64, k *account.AccountKey, systemID types.SystemID, bill *api.Bill, timeout uint64, fcrID, refNo []byte) (*types.TransactionOrder, error) {
	attr := &money.SplitAttributes{
		TargetUnits:    targetUnits,
		RemainingValue: remainingValue,
		Counter:        bill.Counter(),
	}
	txPayload, err := txbuilder.NewTxPayload(systemID, money.PayloadTypeSplit, bill.ID, timeout, fcrID, refNo, attr)
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
	fcrID := mwtypes.FeeCreditRecordIDFormPublicKeyHash(nil, ac.PubKeyHash.Sha256)
	txPayload, err := txbuilder.NewTxPayload(systemID, money.PayloadTypeTransDC, bill.ID, timeout, fcrID, nil, attr)
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
	fcrID := mwtypes.FeeCreditRecordIDFormPublicKeyHash(nil, k.PubKeyHash.Sha256)
	swapTx, err := txbuilder.NewTxPayload(systemID, money.PayloadTypeSwapDC, targetUnitID, timeout, fcrID, nil, attr)
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
	fcrID := mwtypes.FeeCreditRecordIDFormPublicKeyHash(nil, ac.PubKeyHash.Sha256)
	txPayload, err := txbuilder.NewTxPayload(systemID, money.PayloadTypeLock, unitID, timeout, fcrID, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewUnlockTx(ac *account.AccountKey, systemID types.SystemID, b *api.Bill, timeout uint64) (*types.TransactionOrder, error) {
	attr := &money.UnlockAttributes{
		Counter: b.Counter(),
	}
	fcrID := mwtypes.FeeCreditRecordIDFormPublicKeyHash(nil, ac.PubKeyHash.Sha256)
	txPayload, err := txbuilder.NewTxPayload(systemID, money.PayloadTypeUnlock, b.ID, timeout, fcrID, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewLockFCTx(ac *account.AccountKey, systemID types.SystemID, fcb *api.FeeCreditBill, lockStatus uint64, timeout uint64) (*types.TransactionOrder, error) {
	attr := &fc.LockFeeCreditAttributes{
		LockStatus: lockStatus,
		Counter:    fcb.Counter(),
	}
	txPayload, err := txbuilder.NewTxPayload(systemID, fc.PayloadTypeLockFeeCredit, fcb.ID, timeout, nil, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewUnlockFCTx(ac *account.AccountKey, systemID types.SystemID, fcb *api.FeeCreditBill, timeout uint64) (*types.TransactionOrder, error) {
	attr := &fc.UnlockFeeCreditAttributes{
		Counter: fcb.Counter(),
	}
	txPayload, err := txbuilder.NewTxPayload(systemID, fc.PayloadTypeUnlockFeeCredit, fcb.ID, timeout, nil, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewTransferFCTx(amount uint64, targetRecordID []byte, targetUnitCounter *uint64, k *account.AccountKey, moneySystemID, targetSystemID types.SystemID, unitID []byte, counter, timeout, t1, t2 uint64) (*types.TransactionOrder, error) {
	attr := &fc.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: targetSystemID,
		TargetRecordID:         targetRecordID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
		TargetUnitCounter:      targetUnitCounter,
		Counter:                counter,
	}
	txPayload, err := txbuilder.NewTxPayload(moneySystemID, fc.PayloadTypeTransferFeeCredit, unitID, timeout, nil, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, k)
}

func NewAddFCTx(unitID []byte, transferFC *wallet.Proof, ac *account.AccountKey, systemID types.SystemID, timeout uint64) (*types.TransactionOrder, error) {
	attr := &fc.AddFeeCreditAttributes{
		FeeCreditOwnerCondition: templates.NewP2pkh256BytesFromKeyHash(ac.PubKeyHash.Sha256),
		FeeCreditTransfer:       transferFC.TxRecord,
		FeeCreditTransferProof:  transferFC.TxProof,
	}
	txPayload, err := txbuilder.NewTxPayload(systemID, fc.PayloadTypeAddFeeCredit, unitID, timeout, nil, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, ac)
}

func NewCloseFCTx(systemID types.SystemID, unitID []byte, timeout uint64, amount uint64, targetUnitID []byte, targetUnitCounter uint64, k *account.AccountKey) (*types.TransactionOrder, error) {
	attr := &fc.CloseFeeCreditAttributes{
		Amount:            amount,
		TargetUnitID:      targetUnitID,
		TargetUnitCounter: targetUnitCounter,
	}
	txPayload, err := txbuilder.NewTxPayload(systemID, fc.PayloadTypeCloseFeeCredit, unitID, timeout, nil, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, k)
}

func NewReclaimFCTx(systemID types.SystemID, unitID []byte, timeout uint64, fcProof *wallet.Proof, counter uint64, k *account.AccountKey) (*types.TransactionOrder, error) {
	attr := &fc.ReclaimFeeCreditAttributes{
		CloseFeeCreditTransfer: fcProof.TxRecord,
		CloseFeeCreditProof:    fcProof.TxProof,
		Counter:                counter,
	}
	txPayload, err := txbuilder.NewTxPayload(systemID, fc.PayloadTypeReclaimFeeCredit, unitID, timeout, nil, nil, attr)
	if err != nil {
		return nil, err
	}
	return SignPayload(txPayload, k)
}

func SignPayload(payload *types.Payload, ac *account.AccountKey) (*types.TransactionOrder, error) {
	payloadSig, err := txbuilder.SignPayload(payload, ac.PrivKey)
	if err != nil {
		return nil, err
	}
	return txbuilder.NewTransactionOrderP2PKH(payload, payloadSig, ac.PubKey), nil
}
