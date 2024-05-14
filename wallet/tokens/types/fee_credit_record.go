package types

import (
	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
)

func FeeCreditRecordIDFromPublicKey(shardPart, pubKey []byte) types.UnitID {
	ownerPredicate := templates.NewP2pkh256BytesFromKey(pubKey)
	return FeeCreditRecordIDFormOwnerPredicate(shardPart, ownerPredicate)
}

func FeeCreditRecordIDFormPublicKeyHash(shardPart, pubKeyHash []byte) types.UnitID {
	ownerPredicate := templates.NewP2pkh256BytesFromKeyHash(pubKeyHash)
	return FeeCreditRecordIDFormOwnerPredicate(shardPart, ownerPredicate)
}

func FeeCreditRecordIDFormOwnerPredicate(shardPart []byte, ownerPredicate []byte) types.UnitID {
	unitPart := hash.Sum256(ownerPredicate)
	return tokens.NewFeeCreditRecordID(shardPart, unitPart)
}
