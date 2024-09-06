package txbuilder

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

// NewAddVarTx creates a 'addVar' transaction order.
func NewAddVarTx(varData orchestration.ValidatorAssignmentRecord, systemID types.SystemID, unitID types.UnitID, timeout uint64, signingKey *account.AccountKey, maxFee uint64) (*types.TransactionOrder, error) {
	attr := &orchestration.AddVarAttributes{
		Var: varData,
	}

	txPayload, err := sdktypes.NewPayload(systemID, unitID, orchestration.PayloadTypeAddVAR, attr,
		sdktypes.WithTimeout(timeout),
		sdktypes.WithMaxFee(maxFee),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx: %w", err)
	}

	txo := &types.TransactionOrder{Payload: txPayload}
	if signingKey != nil {
		signer, err := crypto.NewInMemorySecp256K1SignerFromKey(signingKey.PrivKey)
		if err != nil {
			return nil, err
		}
		ownerProof, err := sdktypes.NewP2pkhSignature(txo, signer)
		if err != nil {
			return nil, fmt.Errorf("failed to sign tx: %w", err)
		}
		if err = txo.SetAuthProof(orchestration.AddVarAuthProof{OwnerProof: ownerProof}); err != nil {
			return nil, fmt.Errorf("failed to set auth proof: %w", err)
		}
	}
	return txo, nil
}
