package dc

import (
	"context"
	"crypto"
	"fmt"
	"log/slog"
	"sort"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/txbuilder"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

type (
	DustCollector struct {
		systemID      types.SystemID
		maxBillsPerDC int
		txTimeout     uint64
		moneyClient   RpcClient
		log           *slog.Logger
	}

	DustCollectionResult struct {
		SwapProof *wallet.Proof
		LockProof *wallet.Proof
	}

	RpcClient interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetBill(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.Bill, error)
		GetFeeCreditRecord(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error)
		GetUnitsByOwnerID(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error)
		SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		GetTransactionProof(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error)
	}
)

func NewDustCollector(systemID types.SystemID, maxBillsPerDC int, txTimeout uint64, moneyClient RpcClient, log *slog.Logger) *DustCollector {
	return &DustCollector{
		systemID:      systemID,
		maxBillsPerDC: maxBillsPerDC,
		txTimeout:     txTimeout,
		moneyClient:   moneyClient,
		log:           log,
	}
}

// CollectDust joins up to N units into existing target unit, prioritizing smallest units first. The largest unit is
// selected as the target unit. Returns swap transaction proof or error or nil if there's not enough bills to swap.
func (w *DustCollector) CollectDust(ctx context.Context, accountKey *account.AccountKey) (*DustCollectionResult, error) {
	return w.runDustCollection(ctx, accountKey)
}

// runDustCollection executes dust collection process.
func (w *DustCollector) runDustCollection(ctx context.Context, accountKey *account.AccountKey) (*DustCollectionResult, error) {
	// fetch non-dc bills
	bills, err := api.FetchBills(ctx, w.moneyClient, accountKey.PubKeyHash.Sha256)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}

	// filter any locked bills
	bills, _ = util.FilterSlice(bills, func(b *api.Bill) (bool, error) {
		return !b.IsLocked(), nil
	})

	// sort bills by value smallest first
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value() < bills[j].Value()
	})

	// fetch fee credit bill
	fcbID := money.NewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256)
	fcb, err := api.FetchFeeCreditBill(ctx, w.moneyClient, fcbID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit bill: %w", err)
	}

	// verify that we have at least two bills to join
	if len(bills) < 2 {
		w.log.InfoContext(ctx, "account has less than two unlocked bills, skipping dust collection")
		return nil, nil
	}

	// use the largest bill as target
	targetBill := bills[len(bills)-1]
	billCountToSwap := min(w.maxBillsPerDC, len(bills)-1)
	billsToSwap := bills[:billCountToSwap]

	// verify balance
	txsCost := txbuilder.MaxFee * uint64(len(billsToSwap)+2) // +2 for swap and lock tx
	if fcb.Balance() < txsCost {
		return nil, fmt.Errorf("insufficient fee credit balance for transactions: need at least %d Tema "+
			"but have %d Tema to send lock tx, %d dust transfer transactions and swap tx", txsCost, fcb.Balance(), len(billsToSwap))
	}

	// lock target bill
	lockTxSub, err := w.lockTargetBill(ctx, accountKey, targetBill)
	if err != nil {
		return nil, fmt.Errorf("failed to lock target bill: %w", err)
	}

	// exec swap
	return w.submitDCBatch(ctx, accountKey, lockTxSub, targetBill.Counter()+1, billsToSwap)
}

// submitDCBatch creates dust transfers from given bills and locked target bill.
func (w *DustCollector) submitDCBatch(ctx context.Context, k *account.AccountKey, lockTxSub *txsubmitter.TxSubmission, counter uint64, billsToSwap []*api.Bill) (*DustCollectionResult, error) {
	// create dc batch
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, err
	}
	dcBatch := txsubmitter.NewBatch(w.moneyClient, w.log)
	for _, b := range billsToSwap {
		tx, err := txbuilder.NewDustTx(k, w.systemID, b, lockTxSub.UnitID, counter, timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to build dust transfer transaction: %w", err)
		}
		dcBatch.Add(&txsubmitter.TxSubmission{
			UnitID:      tx.UnitID(),
			TxHash:      tx.Hash(crypto.SHA256),
			Transaction: tx,
		})
	}

	// send dc batch
	w.log.InfoContext(ctx, fmt.Sprintf("submitting dc batch of %d dust transfers with target bill %x", len(dcBatch.Submissions()), lockTxSub.UnitID))
	if err := dcBatch.SendTx(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to send dust transfer transactions: %w", err)
	}
	proofs, err := w.extractProofsFromBatch(dcBatch)
	if err != nil {
		return nil, fmt.Errorf("failed to extract proofs from dc batch: %w", err)
	}

	// send swap tx, return swap proof
	swapProof, err := w.swapDCBills(ctx, k, proofs, lockTxSub.UnitID)
	if err != nil {
		return nil, fmt.Errorf("failed to swap dc bills: %w", err)
	}
	return &DustCollectionResult{SwapProof: swapProof, LockProof: lockTxSub.Proof}, nil
}

// swapDCBills creates swap transfer from given dcProofs and target bill, joining the dcBills into the target bill,
// the target bill is expected to be locked on server side.
func (w *DustCollector) swapDCBills(ctx context.Context, k *account.AccountKey, dcProofs []*wallet.Proof, unitID types.UnitID) (*wallet.Proof, error) {
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, err
	}

	// create swap tx
	swapTx, err := txbuilder.NewSwapTx(k, w.systemID, dcProofs, unitID, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap tx: %w", err)
	}

	// create tx submitter batch
	dcBatch := txsubmitter.NewBatch(w.moneyClient, w.log)
	sub := &txsubmitter.TxSubmission{
		UnitID:      swapTx.UnitID(),
		TxHash:      swapTx.Hash(crypto.SHA256),
		Transaction: swapTx,
	}
	dcBatch.Add(sub)

	// send swap tx
	w.log.InfoContext(ctx, fmt.Sprintf("sending swap tx with timeout=%d, unitID=%X", timeout, unitID))
	if err := dcBatch.SendTx(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to send swap tx: %w", err)
	}
	return sub.Proof, nil
}

func (w *DustCollector) lockTargetBill(ctx context.Context, k *account.AccountKey, targetBill *api.Bill) (*txsubmitter.TxSubmission, error) {
	// create lock tx
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, err
	}
	lockTx, err := txbuilder.NewLockTx(k, w.systemID, targetBill.ID, targetBill.Counter(), wallet.LockReasonCollectDust, timeout)
	if err != nil {
		return nil, err
	}
	// lock target bill server side
	w.log.InfoContext(ctx, fmt.Sprintf("locking target bill in node %x", targetBill.ID))
	lockTxHash := lockTx.Hash(crypto.SHA256)
	lockTxBatch := txsubmitter.NewBatch(w.moneyClient, w.log)
	lockTxBatch.Add(&txsubmitter.TxSubmission{
		UnitID:      lockTx.UnitID(),
		TxHash:      lockTxHash,
		Transaction: lockTx,
	})
	if err := lockTxBatch.SendTx(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to send or confirm lock tx: %w", err)
	}
	return lockTxBatch.Submissions()[0], nil
}

func (w *DustCollector) extractProofsFromBatch(dcBatch *txsubmitter.TxSubmissionBatch) ([]*wallet.Proof, error) {
	var proofs []*wallet.Proof
	for _, sub := range dcBatch.Submissions() {
		proofs = append(proofs, sub.Proof)
	}
	return proofs, nil
}

func (w *DustCollector) getTxTimeout(ctx context.Context) (uint64, error) {
	roundNumber, err := w.moneyClient.GetRoundNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch round number: %w", err)
	}
	return roundNumber + w.txTimeout, nil
}

func (w *DustCollector) Close() error {
	return nil // do nothing
}

// GetFeeSum sums spent fees from the result
func (d *DustCollectionResult) GetFeeSum() (uint64, error) {
	if d == nil {
		return 0, nil
	}
	var feeSum uint64
	if d.SwapProof != nil {
		feeSum += d.SwapProof.TxRecord.GetActualFee()
		var swapAttr *money.SwapDCAttributes
		if err := d.SwapProof.TxRecord.TransactionOrder.UnmarshalAttributes(&swapAttr); err != nil {
			return 0, fmt.Errorf("failed to unmarshal swap transaction to calculate fee sum: %w", err)
		}
		for _, dcTx := range swapAttr.DcTransfers {
			feeSum += dcTx.GetActualFee()
		}
	}
	if d.LockProof != nil {
		feeSum += d.LockProof.TxRecord.GetActualFee()
	}
	return feeSum, nil
}
