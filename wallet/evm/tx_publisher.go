package evm

import (
	"context"
	"crypto"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/evm/client"
)

type (
	Client interface {
		GetRoundNumber(ctx context.Context) (*client.RoundNumber, error)
		PostTransaction(ctx context.Context, tx *types.TransactionOrder) error
		GetTxProof(ctx context.Context, unitID types.UnitID, txHash sdktypes.TxHash) (*types.TxRecordProof, error)
	}

	TxPublisher struct {
		cli Client
	}
)

func NewTxPublisher(restClient Client) *TxPublisher {
	return &TxPublisher{
		cli: restClient,
	}
}

// SendTx sends a tx and waits for confirmation, returns tx proof
func (w *TxPublisher) SendTx(ctx context.Context, tx *types.TransactionOrder, _ []byte) (*types.TxRecordProof, error) {
	txHash := tx.Hash(crypto.SHA256)
	if err := w.cli.PostTransaction(ctx, tx); err != nil {
		return nil, fmt.Errorf("evm post tx failed: %w", err)
	}
	// confirm transaction
	timeout := tx.Timeout()
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("confirming transaction interrupted: %w", err)
		}
		rnr, err := w.cli.GetRoundNumber(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to read latest round from evm node: %w", err)
		}
		if rnr.LastIndexedRoundNumber >= timeout {
			return nil, fmt.Errorf("confirmation timeout evm round %v, tx timeout round %v", rnr.LastIndexedRoundNumber, timeout)
		}
		proof, err := w.cli.GetTxProof(ctx, tx.GetUnitID(), txHash)
		if err != nil {
			return nil, err
		}
		if proof != nil {
			return proof, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (w *TxPublisher) Close() {
}
