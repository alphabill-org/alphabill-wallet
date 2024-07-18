package txbuilder

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

const MaxFee = uint64(10)

// NewAddVarTx creates a 'addVar' transaction order.
func NewAddVarTx(varData orchestration.ValidatorAssignmentRecord, systemID types.SystemID, unitID types.UnitID, timeout uint64, signingKey *account.AccountKey) (*types.TransactionOrder, error) {
	attr := &orchestration.AddVarAttributes{
		Var: varData,
	}

	opts := &sdktypes.TxOptions{}
	sdktypes.WithTimeout(timeout)(opts)
	sdktypes.WithMaxFee(MaxFee)(opts)

	txPayload, err := sdktypes.NewPayload(systemID, unitID, orchestration.PayloadTypeAddVAR, attr, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx: %w", err)
	}

	tx := &types.TransactionOrder{Payload: txPayload}
	if signingKey != nil {
		ownerProof := sdktypes.NewP2pkhProofGenerator(signingKey.PrivKey, signingKey.PubKey)
		if err := tx.SetOwnerProof(ownerProof); err != nil {
			return nil, fmt.Errorf("failed to sign tx: %w", err)
		}
	}
	return tx, nil
}
