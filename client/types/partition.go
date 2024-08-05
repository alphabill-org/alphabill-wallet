package types

import (
	"context"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/client/tx"
)

type (
	PartitionClient interface {
		GetNodeInfo(ctx context.Context) (*NodeInfoResponse, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*Proof, error)
		GetTransactionProof(ctx context.Context, txHash types.Bytes) (*Proof, error)
		GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (FeeCreditRecord, error)
		Close()
	}

	FeeCreditRecord interface {
		SystemID() types.SystemID
		ID() types.UnitID
		Balance() uint64
		Counter() *uint64
		Timeout() uint64
		LockStatus() uint64

		AddFeeCredit(ownerPredicate []byte, transFCProof *Proof, txOptions ...tx.Option) (*types.TransactionOrder, error)
		CloseFeeCredit(targetBillID types.UnitID, targetBillCounter uint64, txOptions ...tx.Option) (*types.TransactionOrder, error)
		Lock(lockStatus uint64, txOptions ...tx.Option) (*types.TransactionOrder, error)
		Unlock(txOptions ...tx.Option) (*types.TransactionOrder, error)
	}

	NodeInfoResponse struct {
		SystemID            types.SystemID `json:"systemId"` // hex encoded system identifier
		Name                string         `json:"name"`     // one of [money node | tokens node | evm node]
		Self                PeerInfo       `json:"self"`     // information about this peer
		BootstrapNodes      []PeerInfo     `json:"bootstrapNodes"`
		RootValidators      []PeerInfo     `json:"rootValidators"`
		PartitionValidators []PeerInfo     `json:"partitionValidators"`
		OpenConnections     []PeerInfo     `json:"openConnections"` // all libp2p connections to other peers in the network
	}

	PeerInfo struct {
		Identifier string   `json:"identifier"`
		Addresses  []string `json:"addresses"`
	}

	// Proof wrapper struct around TxRecord and TxProof
	Proof struct {
		_        struct{}                 `cbor:",toarray"`
		TxRecord *types.TransactionRecord `json:"txRecord"`
		TxProof  *types.TxProof           `json:"txProof"`
	}
)

func (p *Proof) GetActualFee() uint64 {
	if p == nil {
		return 0
	}
	return p.TxRecord.GetActualFee()
}
