package rpc

import "github.com/alphabill-org/alphabill/types"

type (
	Bill struct {
		UnitID         types.UnitID
		Value          uint64
		LastUpdated    uint64
		Backlink       []byte
		Locked         uint64
		OwnerPredicate []byte
	}

	FeeCreditRecord struct {
		UnitID         types.UnitID
		OwnerPredicate []byte
	}
)
