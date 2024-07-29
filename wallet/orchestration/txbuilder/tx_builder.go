package txbuilder

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/client/tx"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

const MaxFee = uint64(10)

// NewAddVarTx creates a 'addVar' transaction order.
func NewAddVarTx(varData orchestration.ValidatorAssignmentRecord, systemID types.SystemID, unitID types.UnitID, timeout uint64, signingKey *account.AccountKey) (*types.TransactionOrder, error) {
	attr := &orchestration.AddVarAttributes{
		Var: varData,
	}

	opts := &tx.TxOptions{}
	tx.WithTimeout(timeout)(opts)
	tx.WithMaxFee(MaxFee)(opts)

	txPayload, err := tx.NewPayload(systemID, unitID, orchestration.PayloadTypeAddVAR, attr, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx: %w", err)
	}

	txo := &types.TransactionOrder{Payload: txPayload}
	if signingKey != nil {
		ownerProof := tx.NewP2pkhProofGenerator(signingKey.PrivKey, signingKey.PubKey)
		if err := txo.SetOwnerProof(ownerProof); err != nil {
			return nil, fmt.Errorf("failed to sign tx: %w", err)
		}
	}
	return txo, nil
}
