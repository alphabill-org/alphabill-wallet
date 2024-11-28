package txsubmitter

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

type (
	TxSubmission struct {
		UnitID      types.UnitID
		TxHash      hex.Bytes
		Transaction *types.TransactionOrder
		Proof       *types.TxRecordProof
	}

	TxSubmissionBatch struct {
		submissions     []*TxSubmission
		maxTimeout      uint64
		partitionClient sdktypes.PartitionClient
		log             *slog.Logger
	}
)

func New(tx *types.TransactionOrder) (*TxSubmission, error) {
	txHash, err := tx.Hash(crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to hash tx: %w", err)
	}
	return &TxSubmission{
		UnitID:      tx.GetUnitID(),
		TxHash:      txHash,
		Transaction: tx,
	}, nil
}

func (s *TxSubmission) ToBatch(partitionClient sdktypes.PartitionClient, log *slog.Logger) *TxSubmissionBatch {
	return &TxSubmissionBatch{
		partitionClient: partitionClient,
		submissions:     []*TxSubmission{s},
		maxTimeout:      s.Transaction.Timeout(),
		log:             log,
	}
}

func (s *TxSubmission) Confirmed() bool {
	return s.Proof != nil
}

func NewBatch(partitionClient sdktypes.PartitionClient, log *slog.Logger) *TxSubmissionBatch {
	return &TxSubmissionBatch{
		partitionClient: partitionClient,
		log:             log,
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
		_, err := t.partitionClient.SendTransaction(ctx, txSubmission.Transaction)
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

		roundNumber, err := t.partitionClient.GetRoundNumber(ctx)
		if err != nil {
			return err
		}
		unconfirmed := false
		failed := false
		for _, sub := range t.submissions {
			if sub.Confirmed() {
				continue
			}
			if roundNumber <= sub.Transaction.Timeout() {
				proof, err := t.partitionClient.GetTransactionProof(ctx, sub.TxHash)
				if err != nil {
					return err
				}
				if proof != nil {
					sub.Proof = proof

					var status types.TxStatus
					if proof.TxRecord != nil && proof.TxRecord.ServerMetadata != nil {
						status = proof.TxRecord.ServerMetadata.SuccessIndicator
					}
					switch status {
					case types.TxStatusSuccessful:
						t.log.DebugContext(ctx, fmt.Sprintf("Tx confirmed: hash=%X, unitID=%s", sub.TxHash, sub.UnitID))
					case types.TxErrOutOfGas:
						t.log.InfoContext(ctx, fmt.Sprintf("Tx failed: out of gas: hash=%X, unitID=%s", sub.TxHash, sub.UnitID))
						failed = true
					case types.TxStatusFailed:
						t.log.InfoContext(ctx, fmt.Sprintf("Tx failed: hash=%X, unitID=%s", sub.TxHash, sub.UnitID))
						failed = true
					}
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
						t.log.InfoContext(ctx, fmt.Sprintf("Tx not confirmed: hash=%X, unitID=%s", sub.TxHash, sub.UnitID))
					}
				}
				return errors.New("confirmation timeout")
			}

			time.Sleep(500 * time.Millisecond)
		} else if failed {
			return errors.New("transaction(s) failed")
		} else {
			t.log.InfoContext(ctx, "All transactions confirmed")
			return nil
		}
	}
}
