package txsubmitter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/types"

	"github.com/alphabill-org/alphabill-wallet/wallet"
)

type (
	TxSubmission struct {
		UnitID      types.UnitID
		TxHash      types.Bytes
		Transaction *types.TransactionOrder
		Proof       *wallet.Proof
	}

	TxSubmissionBatch struct {
		sender      wallet.PubKey
		submissions []*TxSubmission
		maxTimeout  uint64
		rpcClient   RpcClient
		log         *slog.Logger
	}

	RpcClient interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		GetTransactionProof(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error)
	}
)

func (s *TxSubmission) ToBatch(backend RpcClient, sender wallet.PubKey, log *slog.Logger) *TxSubmissionBatch {
	return &TxSubmissionBatch{
		sender:      sender,
		rpcClient:   backend,
		submissions: []*TxSubmission{s},
		maxTimeout:  s.Transaction.Timeout(),
		log:         log,
	}
}

func (s *TxSubmission) Confirmed() bool {
	return s.Proof != nil
}

func NewBatch(sender wallet.PubKey, rpcClient RpcClient, log *slog.Logger) *TxSubmissionBatch {
	return &TxSubmissionBatch{
		sender:    sender,
		rpcClient: rpcClient,
		log:       log,
	}
}

func (t *TxSubmissionBatch) Add(sub *TxSubmission) {
	t.submissions = append(t.submissions, sub)
	if sub.Transaction.Timeout() > t.maxTimeout {
		t.maxTimeout = sub.Transaction.Timeout()
	}
}

func (t *TxSubmissionBatch) Submissions() []*TxSubmission {
	return t.submissions
}

func (t *TxSubmissionBatch) transactions() []*types.TransactionOrder {
	txs := make([]*types.TransactionOrder, 0, len(t.submissions))
	for _, sub := range t.submissions {
		txs = append(txs, sub.Transaction)
	}
	return txs
}

func (t *TxSubmissionBatch) SendTx(ctx context.Context, confirmTx bool) error {
	if len(t.submissions) == 0 {
		return errors.New("no transactions to send")
	}
	for _, txSubmission := range t.submissions {
		_, err := t.rpcClient.SendTransaction(ctx, txSubmission.Transaction)
		if err != nil {
			return err
		}
	}
	if confirmTx {
		return t.confirmUnitsTx(ctx)
	}
	return nil
}

func (t *TxSubmissionBatch) confirmUnitsTx(ctx context.Context) error {
	t.log.InfoContext(ctx, "Confirming submitted transactions")

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("confirming transactions interrupted: %w", err)
		}

		roundNumber, err := t.rpcClient.GetRoundNumber(ctx)
		if err != nil {
			return err
		}
		unconfirmed := false
		for _, sub := range t.submissions {
			if sub.Confirmed() {
				continue
			}
			if roundNumber <= sub.Transaction.Timeout() {
				txRecord, txProof, err := t.rpcClient.GetTransactionProof(ctx, sub.TxHash)
				if err != nil && !strings.Contains(err.Error(), "index not found") { // TODO type safe error check
					return err
				}
				if txRecord != nil && txProof != nil {
					t.log.DebugContext(ctx, "Unit is confirmed", logger.UnitID(sub.UnitID))
					sub.Proof = &wallet.Proof{TxRecord: txRecord, TxProof: txProof}
				}
			}

			unconfirmed = unconfirmed || !sub.Confirmed()
		}
		if unconfirmed {
			// If this was the last attempt to get proofs, log the ones that timed out.
			if roundNumber > t.maxTimeout {
				t.log.InfoContext(ctx, "Tx confirmation timeout is reached", logger.Round(roundNumber))
				for _, sub := range t.submissions {
					if !sub.Confirmed() {
						t.log.InfoContext(ctx, fmt.Sprintf("Tx not confirmed for hash=%X", sub.TxHash), logger.UnitID(sub.UnitID))
					}
				}
				return errors.New("confirmation timeout")
			}

			time.Sleep(500 * time.Millisecond)
		} else {
			t.log.InfoContext(ctx, "All transactions confirmed")
			return nil
		}
	}
}
