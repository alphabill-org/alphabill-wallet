package dc

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/txbuilder"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

type (
	DustCollector struct {
		systemID      types.SystemID
		maxBillsPerDC int
		txTimeout     uint64
		moneyClient   sdktypes.MoneyPartitionClient
		log           *slog.Logger
	}

	DustCollectionResult struct {
		SwapProof *sdktypes.Proof
		LockProof *sdktypes.Proof
	}
)

func NewDustCollector(systemID types.SystemID, maxBillsPerDC int, txTimeout uint64, moneyClient sdktypes.MoneyPartitionClient, log *slog.Logger) *DustCollector {
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
	bills, err := w.moneyClient.GetBills(ctx, accountKey.PubKeyHash.Sha256)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}

	// filter any locked bills
	bills, _ = util.FilterSlice(bills, func(b sdktypes.Bill) (bool, error) {
		return b.LockStatus() == 0, nil
	})

	// sort bills by value smallest first
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value() < bills[j].Value()
	})

	// verify that we have at least two bills to join
	if len(bills) < 2 {
		w.log.InfoContext(ctx, "account has less than two unlocked bills, skipping dust collection")
		return nil, nil
	}

	// fetch fee credit bill
	fcb, err := w.moneyClient.GetFeeCreditRecordByOwnerID(ctx, accountKey.PubKeyHash.Sha256)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit bill: %w", err)
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
	lockTxSub, err := w.lockTargetBill(ctx, accountKey, targetBill, fcb.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to lock target bill: %w", err)
	}
	// lock transaction confirmed, counter was increased
	targetBill.IncreaseCounter()

	// exec swap (increment counter for successful lock transaction)
	return w.submitDCBatch(ctx, accountKey, fcb.ID, lockTxSub, targetBill, billsToSwap)
}

// submitDCBatch creates dust transfers from given bills and locked target bill.
func (w *DustCollector) submitDCBatch(ctx context.Context, k *account.AccountKey, fcrID []byte, lockTxSub *txsubmitter.TxSubmission, targetBill sdktypes.Bill, billsToSwap []sdktypes.Bill) (*DustCollectionResult, error) {
	// create dc batch
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, err
	}
	dcBatch := txsubmitter.NewBatch(w.moneyClient, w.log)
	for _, b := range billsToSwap {
		tx, err := b.TransferToDustCollector(targetBill,
			sdktypes.WithTimeout(timeout),
			sdktypes.WithFeeCreditRecordID(fcrID),
			sdktypes.WithOwnerProof(sdktypes.NewP2pkhProofGenerator(k.PrivKey, k.PubKey)))
		if err != nil {
			return nil, fmt.Errorf("failed to build dust transfer transaction: %w", err)
		}
		dcBatch.Add(txsubmitter.New(tx))
	}

	// send dc batch
	w.log.InfoContext(ctx, fmt.Sprintf("submitting dc batch of %d dust transfers with target bill %x", len(dcBatch.Submissions()), targetBill.ID))
	if err := dcBatch.SendTx(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to send dust transfer transactions: %w", err)
	}
	proofs, err := w.extractProofsFromBatch(dcBatch)
	if err != nil {
		return nil, fmt.Errorf("failed to extract proofs from dc batch: %w", err)
	}

	// send swap tx, return swap proof
	swapProof, err := w.swapDCBills(ctx, k, proofs, targetBill, fcrID)
	if err != nil {
		return nil, fmt.Errorf("failed to swap dc bills: %w", err)
	}
	return &DustCollectionResult{SwapProof: swapProof, LockProof: lockTxSub.Proof}, nil
}

// swapDCBills creates swap transfer from given dcProofs and target bill, joining the dcBills into the target bill,
// the target bill is expected to be locked on server side.
func (w *DustCollector) swapDCBills(ctx context.Context, k *account.AccountKey, dcProofs []*sdktypes.Proof, targetBill sdktypes.Bill, fcrID []byte) (*sdktypes.Proof, error) {
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, err
	}

	// create swap tx
	swapTx, err  := targetBill.SwapWithDustCollector(dcProofs,
		sdktypes.WithTimeout(timeout),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithOwnerProof(sdktypes.NewP2pkhProofGenerator(k.PrivKey, k.PubKey)))
	if err != nil {
		return nil, fmt.Errorf("failed to build swap tx: %w", err)
	}

	// create tx submitter batch
	dcBatch := txsubmitter.NewBatch(w.moneyClient, w.log)
	sub := txsubmitter.New(swapTx)
	dcBatch.Add(sub)

	// send swap tx
	w.log.InfoContext(ctx, fmt.Sprintf("sending swap tx with timeout=%d, unitID=%X", timeout, targetBill.ID()))
	if err := dcBatch.SendTx(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to send swap tx: %w", err)
	}
	return sub.Proof, nil
}

func (w *DustCollector) lockTargetBill(ctx context.Context, k *account.AccountKey, targetBill sdktypes.Bill, fcrID types.UnitID) (*txsubmitter.TxSubmission, error) {
	// create lock tx
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, err
	}
	lockTx, err := targetBill.Lock(
		wallet.LockReasonCollectDust,
		sdktypes.WithTimeout(timeout),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithOwnerProof(sdktypes.NewP2pkhProofGenerator(k.PrivKey, k.PubKey)))
	if err != nil {
		return nil, err
	}

	// lock target bill server side
	w.log.InfoContext(ctx, fmt.Sprintf("locking target bill in node %x", targetBill.ID))
	lockTxBatch := txsubmitter.NewBatch(w.moneyClient, w.log)
	lockTxBatch.Add(txsubmitter.New(lockTx))
	if err := lockTxBatch.SendTx(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to send or confirm lock tx: %w", err)
	}
	return lockTxBatch.Submissions()[0], nil
}

func (w *DustCollector) extractProofsFromBatch(dcBatch *txsubmitter.TxSubmissionBatch) ([]*sdktypes.Proof, error) {
	var proofs []*sdktypes.Proof
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
