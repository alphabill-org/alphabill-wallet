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

	FeeCreditRecord struct {
		SystemID   types.SystemID
		ID         types.UnitID
		Balance    uint64
		Timeout    uint64
		LockStatus uint64
		Counter    *uint64
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

func (f *FeeCreditRecord) AddFeeCredit(ownerPredicate []byte, transFCProof *Proof, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.AddFeeCreditAttributes{
		FeeCreditOwnerCondition: ownerPredicate,
		FeeCreditTransfer:       transFCProof.TxRecord,
		FeeCreditTransferProof:  transFCProof.TxProof,
	}
	txPayload, err := NewPayload(f.SystemID, f.ID, fc.PayloadTypeAddFeeCredit, attr, txOptions...)
	if err != nil {
		return nil, err
	}
	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, nil, nil, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (f *FeeCreditRecord) CloseFeeCredit(targetBillID types.UnitID, targetBillCounter uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.CloseFeeCreditAttributes{
		Amount:            f.Balance,
		TargetUnitID:      targetBillID,
		TargetUnitCounter: targetBillCounter,
		Counter:           *f.Counter,
	}
	txPayload, err := NewPayload(f.SystemID, f.ID, fc.PayloadTypeCloseFeeCredit, attr, txOptions...)
	if err != nil {
		return nil, err
	}
	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, nil, nil, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (f *FeeCreditRecord) Lock(lockStatus uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.LockFeeCreditAttributes{
		LockStatus: lockStatus,
		Counter:    *f.Counter,
	}
	txPayload, err := NewPayload(f.SystemID, f.ID, fc.PayloadTypeLockFeeCredit, attr, txOptions...)
	if err != nil {
		return nil, err
	}
	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, nil, nil, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (f *FeeCreditRecord) Unlock(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.UnlockFeeCreditAttributes{
		Counter: *f.Counter,
	}
	txPayload, err := NewPayload(f.SystemID, f.ID, fc.PayloadTypeUnlockFeeCredit, attr, txOptions...)
	if err != nil {
		return nil, err
	}
	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, nil, nil, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (p *Proof) GetActualFee() uint64 {
	if p == nil {
		return 0
	}
	return p.TxRecord.GetActualFee()
}
