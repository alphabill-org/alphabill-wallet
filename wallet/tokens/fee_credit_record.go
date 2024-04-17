package tokens

import (
	"github.com/alphabill-org/alphabill-go-sdk/hash"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
)

func FeeCreditRecordIDFromPublicKey(shardPart, pubKey []byte) types.UnitID {
	unitPart := hash.Sum256(pubKey)
	return tokens.NewFeeCreditRecordID(shardPart, unitPart)
}
