package txsubmitter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
)

type (
	TxSubmission struct {
		UnitID      types.UnitID
		TxHash      types.Bytes
		Transaction *types.TransactionOrder
		Proof       *wallet.Proof
	}

	TxSubmissionBatch struct {
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

func (s *TxSubmission) ToBatch(rpcClient RpcClient, log *slog.Logger) *TxSubmissionBatch {
	return &TxSubmissionBatch{
		rpcClient:   rpcClient,
		submissions: []*TxSubmission{s},
		maxTimeout:  s.Transaction.Timeout(),
		log:         log,
	}
}

func (s *TxSubmission) Confirmed() bool {
	return s.Proof != nil
}

func NewBatch(rpcClient RpcClient, log *slog.Logger) *TxSubmissionBatch {
	return &TxSubmissionBatch{
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
				if err != nil && !errors.Is(err, api.ErrNotFound) {
					return err
				}
				if txRecord != nil && txProof != nil {
					t.log.DebugContext(ctx, fmt.Sprintf("Tx confirmed: hash=%X, unitID=%X", sub.TxHash, sub.UnitID))
					sub.Proof = &wallet.Proof{TxRecord: txRecord, TxProof: txProof}
				}
			}

			unconfirmed = unconfirmed || !sub.Confirmed()
		}
		if unconfirmed {
			// If this was the last attempt to get proofs, log the ones that timed out.
			if roundNumber > t.maxTimeout {
				t.log.InfoContext(ctx, fmt.Sprintf("Tx confirmation timeout is reached: round=%d", roundNumber))

				for _, sub := range t.submissions {
					if !sub.Confirmed() {
						t.log.InfoContext(ctx, fmt.Sprintf("Tx not confirmed: hash=%X, unitID=%X", sub.TxHash, sub.UnitID))
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
