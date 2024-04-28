package client

import (
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/evm"
)

type Result struct {
	Success   bool
	ActualFee uint64
	Details   *evm.ProcessingDetails
}
