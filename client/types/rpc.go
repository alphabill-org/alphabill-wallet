package types

import (
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	Unit[T any] struct {
		UnitID         types.UnitID          `json:"unitId"`
		Data           T                     `json:"data"`
		OwnerPredicate types.Bytes           `json:"ownerPredicate,omitempty"`
		StateProof     *types.UnitStateProof `json:"stateProof,omitempty"`
	}

	TransactionRecordAndProof struct {
		TxRecord types.Bytes `json:"txRecord"`
		TxProof  types.Bytes `json:"txProof"`
	}
)
