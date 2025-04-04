package types

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
)

type (
	Unit[T any] struct {
		NetworkID   types.NetworkID       `json:"networkId"`
		PartitionID types.PartitionID     `json:"partitionId"`
		UnitID      types.UnitID          `json:"unitId"`
		Data        T                     `json:"data"`
		StateProof  *types.UnitStateProof `json:"stateProof,omitempty"`
		StateLockTx hex.Bytes             `json:"stateLockTx,omitempty"`
	}

	TransactionRecordAndProof struct {
		TxRecordProof hex.Bytes `json:"txRecordProof"`
	}
)

func (t *TransactionRecordAndProof) ToBaseType() (*types.TxRecordProof, error) {
	if t == nil {
		return nil, errors.New("transaction record and proof must not be nil")
	}
	var txRecordProof *types.TxRecordProof
	if err := types.Cbor.Unmarshal(t.TxRecordProof, &txRecordProof); err != nil {
		return nil, fmt.Errorf("failed to decode tx record: %w", err)
	}
	return txRecordProof, nil
}
