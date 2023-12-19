package testutil

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/predicates/templates"
	moneytx "github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

type InitialBill struct{
	ID    types.UnitID
	Value uint64
	Owner predicates.PredicateBytes
}

func MoneyGenesisState(t *testing.T, initialBill *InitialBill, dustCollectorMoneySupply uint64, sdrs []*genesis.SystemDescriptionRecord) *state.State {
	s := state.NewEmptyState()
	zeroHash := make([]byte, crypto.SHA256.Size())

	// initial bill
	require.NoError(t, s.Apply(state.AddUnit(initialBill.ID, initialBill.Owner, &money.BillData{V: initialBill.Value})))
	_, err := s.AddUnitLog(initialBill.ID, zeroHash)
	require.NoError(t, err)

	// dust collector money supply
	require.NoError(t, s.Apply(state.AddUnit(money.DustCollectorMoneySupplyID, money.DustCollectorPredicate, &money.BillData{V: dustCollectorMoneySupply})))
	_, err = s.AddUnitLog(money.DustCollectorMoneySupplyID, zeroHash)
	require.NoError(t, err)

	// fee credit bills
	for _, sdr := range sdrs {
		fcb := sdr.FeeCreditBill
		require.NoError(t, s.Apply(state.AddUnit(fcb.UnitId, fcb.OwnerPredicate, &money.BillData{})))
		_, err = s.AddUnitLog(fcb.UnitId, zeroHash)
		require.NoError(t, err)
	}

	_, _, err = s.CalculateRoot()
	require.NoError(t, err)

	return s
}

func CreateInitialBillTransferTx(accountKey *account.AccountKey, billID, fcrID types.UnitID, billValue uint64, timeout uint64, backlink []byte) (*types.TransactionOrder, error) {
	attr := &moneytx.TransferAttributes{
		NewBearer:   templates.NewP2pkh256BytesFromKey(accountKey.PubKey),
		TargetValue: billValue,
		Backlink:    backlink,
	}
	attrBytes, err := cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	txo := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   []byte{0, 0, 0, 0},
			Type:       moneytx.PayloadTypeTransfer,
			UnitID:     billID,
			Attributes: attrBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: 1,
				FeeCreditRecordID: fcrID,
			},
		},
	}
	signer, _ := abcrypto.NewInMemorySecp256K1SignerFromKey(accountKey.PrivKey)
	sigBytes, err := txo.PayloadBytes()
	sigData, _ := signer.SignBytes(sigBytes)
	txo.OwnerProof = templates.NewP2pkh256SignatureBytes(sigData, accountKey.PubKey)
	return txo, nil
}
