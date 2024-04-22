package testutil

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/hash"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

type MoneyGenesisConfig struct {
	InitialBillID      types.UnitID
	InitialBillValue   uint64
	InitialBillOwner   types.PredicateBytes
	DCMoneySupplyValue uint64
	SDRs               []*types.SystemDescriptionRecord
}

var (
	defaultMoneySDR = &types.SystemDescriptionRecord{
		SystemIdentifier: money.DefaultSystemID,
		T2Timeout:        2500,
		FeeCreditBill: &types.FeeCreditBill{
			UnitID:         money.NewBillID(nil, []byte{2}),
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}

	// TODO: duplicated from alphabill repo, really need those here?
	// The ID of the dust collector money supply
	DustCollectorMoneySupplyID = money.NewBillID(nil, nil)

	// Dust collector predicate
	DustCollectorPredicate = templates.NewP2pkh256BytesFromKeyHash(hash.Sum256([]byte("dust collector")))
)

func MoneyGenesisState(t *testing.T, config *MoneyGenesisConfig) *state.State {
	if len(config.SDRs) == 0 {
		config.SDRs = append(config.SDRs, defaultMoneySDR)
	}

	s := state.NewEmptyState()
	zeroHash := make([]byte, crypto.SHA256.Size())

	// initial bill
	require.NoError(t, s.Apply(state.AddUnit(config.InitialBillID, config.InitialBillOwner, &money.BillData{V: config.InitialBillValue})))
	require.NoError(t, s.AddUnitLog(config.InitialBillID, zeroHash))

	// dust collector money supply
	require.NoError(t, s.Apply(state.AddUnit(DustCollectorMoneySupplyID, DustCollectorPredicate, &money.BillData{V: config.DCMoneySupplyValue})))
	require.NoError(t, s.AddUnitLog(DustCollectorMoneySupplyID, zeroHash))

	// fee credit bills
	for _, sdr := range config.SDRs {
		fcb := sdr.FeeCreditBill
		require.NoError(t, s.Apply(state.AddUnit(fcb.UnitID, fcb.OwnerPredicate, &money.BillData{})))
		require.NoError(t, s.AddUnitLog(fcb.UnitID, zeroHash))
	}

	_, _, err := s.CalculateRoot()
	require.NoError(t, err)

	return s
}

func CreateInitialBillTransferTx(accountKey *account.AccountKey, billID, fcrID types.UnitID, billValue, timeout, counter uint64) (*types.TransactionOrder, error) {
	attr := &money.TransferAttributes{
		NewBearer:   templates.NewP2pkh256BytesFromKey(accountKey.PubKey),
		TargetValue: billValue,
		Counter:     counter,
	}
	attrBytes, err := types.Cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	txo := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   money.DefaultSystemID,
			Type:       money.PayloadTypeTransfer,
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
	if err != nil {
		return nil, err
	}
	sigData, _ := signer.SignBytes(sigBytes)
	txo.OwnerProof = templates.NewP2pkh256SignatureBytes(sigData, accountKey.PubKey)
	return txo, nil
}
