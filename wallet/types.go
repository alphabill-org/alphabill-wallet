package wallet

const (
	LockReasonAddFees = 1 + iota
	LockReasonReclaimFees
	LockReasonCollectDust
	LockReasonManual
)

type (
	LockReason uint64
)

func (r LockReason) String() string {
	switch r {
	case 0:
		return "unlocked"
	case LockReasonAddFees:
		return "locked for adding fees"
	case LockReasonReclaimFees:
		return "locked for reclaiming fees"
	case LockReasonCollectDust:
		return "locked for dust collection"
	case LockReasonManual:
		return "manually locked by user"
	}
	return "locked"
}
