package types

import (
	"context"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	PartitionClient interface {
		GetNodeInfo(ctx context.Context) (*NodeInfoResponse, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*Proof, error)
		GetTransactionProof(ctx context.Context, txHash types.Bytes) (*Proof, error)
		GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*FeeCreditRecord, error)
		Close()
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

	FeeCreditRecord struct {
		ID   types.UnitID
		Data *fc.FeeCreditRecord
	}

	// Proof wrapper struct around TxRecord and TxProof
	Proof struct {
		_        struct{}                 `cbor:",toarray"`
		TxRecord *types.TransactionRecord `json:"txRecord"`
		TxProof  *types.TxProof           `json:"txProof"`
	}
)

func (b *FeeCreditRecord) IsLocked() bool {
	if b == nil {
		return false
	}
	return b.Data.IsLocked()
}

func (b *FeeCreditRecord) Counter() uint64 {
	if b == nil {
		return 0
	}
	return b.Data.GetCounter()
}

func (b *FeeCreditRecord) Balance() uint64 {
	if b == nil {
		return 0
	}
	if b.Data == nil {
		return 0
	}
	return b.Data.Balance
}

func (p *Proof) GetActualFee() uint64 {
	if p == nil {
		return 0
	}
	return p.TxRecord.GetActualFee()
}
