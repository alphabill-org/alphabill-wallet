package wallet

import (
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"github.com/holiman/uint256"
)

type bill struct {
	Id     *uint256.Int `json:"id"`
	Value  uint64       `json:"value"`
	TxHash []byte       `json:"txHash"`

	// dc bill specific fields
	IsDcBill bool                     `json:"dcBill"`
	DcTx     *transaction.Transaction `json:"dcTx"`
	DcNonce  []byte                   `json:"dcNonce"`
}

// getId returns 32-byte big endian array of bill ids
func (b *bill) getId() []byte {
	bytes32 := b.Id.Bytes32()
	return bytes32[:]
}
