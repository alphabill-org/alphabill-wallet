package testutils

import (
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
)

var (
	DefaultInitialBillID = money.NewBillID(nil, []byte{1})
)
