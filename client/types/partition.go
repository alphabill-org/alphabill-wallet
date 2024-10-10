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
		ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*types.TxRecordProof, error)
		GetTransactionProof(ctx context.Context, txHash types.Bytes) (*types.TxRecordProof, error)
		GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*FeeCreditRecord, error)
		Close()
	}

	FeeCreditRecord struct {
		NetworkID  types.NetworkID
		SystemID   types.SystemID
		ID         types.UnitID
		Balance    uint64
		Timeout    uint64
		LockStatus uint64
		Counter    *uint64
	}

	NodeInfoResponse struct {
		NetworkID           types.NetworkID `json:"networkId"` // hex encoded network identifier
		SystemID            types.SystemID  `json:"systemId"`  // hex encoded system identifier
		Name                string          `json:"name"`      // one of [money node | tokens node | evm node]
		Self                PeerInfo        `json:"self"`      // information about this peer
		BootstrapNodes      []PeerInfo      `json:"bootstrapNodes"`
		RootValidators      []PeerInfo      `json:"rootValidators"`
		PartitionValidators []PeerInfo      `json:"partitionValidators"`
		OpenConnections     []PeerInfo      `json:"openConnections"` // all libp2p connections to other peers in the network
	}

	PeerInfo struct {
		Identifier string   `json:"identifier"`
		Addresses  []string `json:"addresses"`
	}
)

func (f *FeeCreditRecord) AddFeeCredit(ownerPredicate []byte, transFCProof *types.TxRecordProof, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.AddFeeCreditAttributes{
		FeeCreditOwnerPredicate: ownerPredicate,
		FeeCreditTransferProof:  transFCProof,
	}
	return NewTransactionOrder(f.NetworkID, f.SystemID, f.ID, fc.TransactionTypeAddFeeCredit, attr, txOptions...)
}

func (f *FeeCreditRecord) CloseFeeCredit(targetBillID types.UnitID, targetBillCounter uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.CloseFeeCreditAttributes{
		Amount:            f.Balance,
		TargetUnitID:      targetBillID,
		TargetUnitCounter: targetBillCounter,
		Counter:           *f.Counter,
	}
	return NewTransactionOrder(f.NetworkID, f.SystemID, f.ID, fc.TransactionTypeCloseFeeCredit, attr, txOptions...)
}

func (f *FeeCreditRecord) Lock(lockStatus uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.LockFeeCreditAttributes{
		LockStatus: lockStatus,
		Counter:    *f.Counter,
	}
	return NewTransactionOrder(f.NetworkID, f.SystemID, f.ID, fc.TransactionTypeLockFeeCredit, attr, txOptions...)
}

func (f *FeeCreditRecord) Unlock(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.UnlockFeeCreditAttributes{
		Counter: *f.Counter,
	}
	return NewTransactionOrder(f.NetworkID, f.SystemID, f.ID, fc.TransactionTypeUnlockFeeCredit, attr, txOptions...)
}
