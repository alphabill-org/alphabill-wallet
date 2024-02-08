package tokens

import (
	"context"
	"crypto"
	"log/slog"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
	"github.com/alphabill-org/alphabill/types"
)

type (
	TxPublisher struct {
		rpcClient txsubmitter.RpcClient
		log       *slog.Logger
	}
)

func NewTxPublisher(rpcClient txsubmitter.RpcClient, log *slog.Logger) *TxPublisher {
	return &TxPublisher{
		rpcClient: rpcClient,
		log:       log,
	}
}

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *TxPublisher) SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*wallet.Proof, error) {
	txSub := &txsubmitter.TxSubmission{
		UnitID:      tx.UnitID(),
		Transaction: tx,
		TxHash:      tx.Hash(crypto.SHA256),
	}
	txBatch := txSub.ToBatch(w.rpcClient, senderPubKey, w.log)
	if err := txBatch.SendTx(ctx, true); err != nil {
		return nil, err
	}
	return txBatch.Submissions()[0].Proof, nil
}

func (w *TxPublisher) Close() {
}
