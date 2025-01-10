package types

import (
	"context"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
)

type (
	PartitionClient interface {
		GetNodeInfo(ctx context.Context) (*NodeInfoResponse, error)
		PartitionDescription(ctx context.Context) (*types.PartitionDescriptionRecord, error)
		GetRoundInfo(ctx context.Context) (*RoundInfo, error)
		SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*types.TxRecordProof, error)
		GetTransactionProof(ctx context.Context, txHash hex.Bytes) (*types.TxRecordProof, error)
		GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*FeeCreditRecord, error)
		Close()
	}

	FeeCreditRecord struct {
		NetworkID      types.NetworkID   `json:"networkId"`
		PartitionID    types.PartitionID `json:"partitionId"`
		ID             types.UnitID      `json:"id"`
		Balance        uint64            `json:"balance"`
		OwnerPredicate hex.Bytes         `json:"ownerPredicate"`
		MinLifetime    uint64            `json:"minLifetime"`
		LockStatus     uint64            `json:"lockStatus"`
		Counter        *uint64           `json:"counter"`
	}

	NodeInfoResponse struct {
		NetworkID           types.NetworkID       `json:"networkId"`       // hex encoded network identifier
		PartitionID         types.PartitionID     `json:"partitionId"`     // hex encoded partition identifier
		PartitionTypeID     types.PartitionTypeID `json:"partitionTypeId"` // hex encoded partition identifier
		PermissionedMode    bool                  `json:"permissionedMode"`
		FeelessMode         bool                  `json:"feelessMode"`
		Self                PeerInfo              `json:"self"` // information about this peer
		BootstrapNodes      []PeerInfo            `json:"bootstrapNodes"`
		RootValidators      []PeerInfo            `json:"rootValidators"`
		PartitionValidators []PeerInfo            `json:"partitionValidators"`
		OpenConnections     []PeerInfo            `json:"openConnections"` // all libp2p connections to other peers in the network
	}

	RoundInfo struct {
		RoundNumber uint64 `json:"roundNumber"`
		Epoch       uint64 `json:"epoch"`
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
	return NewTransactionOrder(f.NetworkID, f.PartitionID, f.ID, fc.TransactionTypeAddFeeCredit, attr, txOptions...)
}

func (f *FeeCreditRecord) SetFeeCredit(ownerPredicate []byte, amount uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &permissioned.SetFeeCreditAttributes{
		OwnerPredicate: ownerPredicate,
		Amount:         amount,
		Counter:        f.Counter,
	}

	return NewTransactionOrder(f.NetworkID, f.PartitionID, f.ID, permissioned.TransactionTypeSetFeeCredit, attr, txOptions...)
}

func (f *FeeCreditRecord) DeleteFeeCredit(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &permissioned.DeleteFeeCreditAttributes{
		Counter: *f.Counter,
	}

	return NewTransactionOrder(f.NetworkID, f.PartitionID, f.ID, permissioned.TransactionTypeDeleteFeeCredit, attr, txOptions...)
}

func (f *FeeCreditRecord) CloseFeeCredit(targetBillID types.UnitID, targetBillCounter uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.CloseFeeCreditAttributes{
		Amount:            f.Balance,
		TargetUnitID:      targetBillID,
		TargetUnitCounter: targetBillCounter,
		Counter:           *f.Counter,
	}
	return NewTransactionOrder(f.NetworkID, f.PartitionID, f.ID, fc.TransactionTypeCloseFeeCredit, attr, txOptions...)
}

func (f *FeeCreditRecord) Lock(lockStatus uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.LockFeeCreditAttributes{
		LockStatus: lockStatus,
		Counter:    *f.Counter,
	}
	return NewTransactionOrder(f.NetworkID, f.PartitionID, f.ID, fc.TransactionTypeLockFeeCredit, attr, txOptions...)
}

func (f *FeeCreditRecord) Unlock(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.UnlockFeeCreditAttributes{
		Counter: *f.Counter,
	}
	return NewTransactionOrder(f.NetworkID, f.PartitionID, f.ID, fc.TransactionTypeUnlockFeeCredit, attr, txOptions...)
}
