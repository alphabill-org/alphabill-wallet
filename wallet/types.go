package wallet

import (
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
)

func NewP2PKHStateLock(pubKeyHash []byte) *types.StateLock {
	return NewStateLock(templates.NewP2pkh256BytesFromKeyHash(pubKeyHash))
}

func NewStateLock(ownerPredicate []byte) *types.StateLock {
	return &types.StateLock{
		ExecutionPredicate: ownerPredicate,
		RollbackPredicate:  ownerPredicate,
	}
}
