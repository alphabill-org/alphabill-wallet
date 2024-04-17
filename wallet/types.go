package wallet

import (
	"github.com/alphabill-org/alphabill-go-sdk/hash"
	"github.com/alphabill-org/alphabill-go-sdk/types"
)

const (
	LockReasonAddFees = 1 + iota
	LockReasonReclaimFees
	LockReasonCollectDust
	LockReasonManual
)

type (
	TxHash     []byte
	Predicate  []byte
	PubKey     []byte
	PubKeyHash []byte

	// Proof wrapper struct around TxRecord and TxProof
	Proof struct {
		_        struct{}                 `cbor:",toarray"`
		TxRecord *types.TransactionRecord `json:"txRecord"`
		TxProof  *types.TxProof           `json:"txProof"`
	}

	LockReason uint64
)

func (pk PubKey) Hash() PubKeyHash {
	return hash.Sum256(pk)
}

func (p *Proof) GetActualFee() uint64 {
	if p == nil {
		return 0
	}
	return p.TxRecord.GetActualFee()
}

func (r LockReason) String() string {
	switch r {
	case LockReasonAddFees:
		return "locked for adding fees"
	case LockReasonReclaimFees:
		return "locked for reclaiming fees"
	case LockReasonCollectDust:
		return "locked for dust collection"
	case LockReasonManual:
		return "manually locked by user"
	}
	return ""
}
