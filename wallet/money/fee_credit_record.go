package money

import (
	"github.com/alphabill-org/alphabill-go-sdk/hash"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	"github.com/alphabill-org/alphabill-go-sdk/types"
)

func FeeCreditRecordIDFormPublicKey(shardPart, pubKey []byte) types.UnitID {
	unitPart := hash.Sum256(pubKey)
	return money.NewFeeCreditRecordID(shardPart, unitPart)
}
