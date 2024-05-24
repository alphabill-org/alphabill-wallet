package txbuilder

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/txbuilder"
)

const MaxFee = uint64(1)

// NewAddVarTx creates a 'addVar' transaction order.
func NewAddVarTx(varData orchestration.ValidatorAssignmentRecord, systemID types.SystemID, unitID types.UnitID, timeout uint64, signingKey *account.AccountKey) (*types.TransactionOrder, error) {
	attr := &orchestration.AddVarAttributes{
		Var: varData,
	}
	txPayload, err := txbuilder.NewTxPayload(systemID, orchestration.PayloadTypeAddVAR, unitID, nil, timeout, nil, attr)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx: %w", err)
	}
	if signingKey != nil {
		payloadSig, err := txbuilder.SignPayload(txPayload, signingKey.PrivKey)
		if err != nil {
			return nil, fmt.Errorf("failed to sign tx: %w", err)
		}
		return txbuilder.NewTransactionOrderP2PKH(txPayload, payloadSig, signingKey.PubKey), nil
	}
	return &types.TransactionOrder{Payload: txPayload}, nil
}
