package testutils

import (
	"github.com/alphabill-org/alphabill/txsystem/money"
)

var (
	DefaultInitialBillID = money.NewBillID(nil, []byte{1})
)
