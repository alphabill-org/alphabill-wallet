package types

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	Unit[T any] struct {
		NetworkID  types.NetworkID       `json:"networkId"`
		SystemID   types.SystemID        `json:"systemId"`
		UnitID     types.UnitID          `json:"unitId"`
		Data       T                     `json:"data"`
		StateProof *types.UnitStateProof `json:"stateProof,omitempty"`
	}

	TransactionRecordAndProof struct {
		TxRecordProof types.Bytes `json:"txRecordProof"`
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
