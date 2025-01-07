package mocksrv

import (
	"context"
	"crypto"
	"fmt"
	"slices"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

type (
	StateServiceMock struct {
		RoundNumber  uint64
		Units        map[string]*sdktypes.Unit[any]
		OwnerUnitIDs map[string][]types.UnitID
		TxProofs     map[string]*sdktypes.TransactionRecordAndProof
		Block        hex.Bytes
		SentTxs      map[string]*types.TransactionOrder
		Err          error
		GetUnitCalls int
	}

	Options struct {
		Err          error
		RoundNumber  uint64
		TxProofs     map[string]*sdktypes.TransactionRecordAndProof
		Units        map[string]*sdktypes.Unit[any]
		OwnerUnits   map[string][]types.UnitID
		InfoResponse *sdktypes.NodeInfoResponse
	}

	Option func(*Options)
)

func NewStateServiceMock(opts ...Option) *StateServiceMock {
	options := &Options{
		TxProofs:   map[string]*sdktypes.TransactionRecordAndProof{},
		Units:      map[string]*sdktypes.Unit[any]{},
		OwnerUnits: map[string][]types.UnitID{},
	}
	for _, option := range opts {
		option(options)
	}
	return &StateServiceMock{
		Err:          options.Err,
		RoundNumber:  options.RoundNumber,
		Units:        options.Units,
		OwnerUnitIDs: options.OwnerUnits,
		TxProofs:     options.TxProofs,
		SentTxs:      map[string]*types.TransactionOrder{},
	}
}

func WithOwnerUnit(ownerPredicate []byte, unit *sdktypes.Unit[any]) Option {
	return func(o *Options) {
		o.Units[string(unit.UnitID)] = unit
		o.OwnerUnits[string(ownerPredicate)] = append(o.OwnerUnits[string(ownerPredicate)], unit.UnitID)
	}
}

func WithUnit(unit *sdktypes.Unit[any]) Option {
	return func(o *Options) {
		o.Units[string(unit.UnitID)] = unit
	}
}

func WithUnits(units ...*sdktypes.Unit[any]) Option {
	return func(o *Options) {
		for _, unit := range units {
			o.Units[string(unit.UnitID)] = unit
		}
	}
}

func WithTxProof(txHash []byte, txProof *sdktypes.TransactionRecordAndProof) Option {
	return func(o *Options) {
		o.TxProofs[string(txHash)] = txProof
	}
}

func WithRoundNumber(roundNumber uint64) Option {
	return func(o *Options) {
		o.RoundNumber = roundNumber
	}
}

func WithError(err error) Option {
	return func(o *Options) {
		o.Err = err
	}
}

func (s *StateServiceMock) GetRoundNumber(ctx context.Context) (hex.Uint64, error) {
	if s.Err != nil {
		return 0, s.Err
	}
	return hex.Uint64(s.RoundNumber), nil
}

func (s *StateServiceMock) GetUnit(unitID types.UnitID, includeStateProof bool) (*sdktypes.Unit[any], error) {
	s.GetUnitCalls += 1
	if s.Err != nil {
		return nil, s.Err
	}
	u, ok := s.Units[string(unitID)]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (s *StateServiceMock) GetUnitsByOwnerID(ownerID hex.Bytes) ([]types.UnitID, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.OwnerUnitIDs[string(ownerID)], nil
}

func (s *StateServiceMock) GetUnits(unitTypeID uint32) ([]types.UnitID, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	var unitIDs []types.UnitID
	for _, unit := range s.Units {
		unitIDs = append(unitIDs, unit.UnitID)
	}
	slices.SortFunc(unitIDs, func(a, b types.UnitID) int {
		return a.Compare(b)
	})
	return unitIDs, nil
}

func (s *StateServiceMock) SendTransaction(ctx context.Context, tx hex.Bytes) (hex.Bytes, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	var txo *types.TransactionOrder
	if err := types.Cbor.Unmarshal(tx, &txo); err != nil {
		return nil, err
	}
	txHash, err := txo.Hash(crypto.SHA256)
	if err != nil {
		return nil, err
	}
	s.SentTxs[string(txHash)] = txo
	return txHash, nil
}

func (s *StateServiceMock) GetTransactionProof(_ context.Context, txHash hex.Bytes) (*sdktypes.TransactionRecordAndProof, error) {
	if s.Err != nil {
		return nil, s.Err
	}

	txProof, ok := s.TxProofs[string(txHash)]
	if ok {
		return txProof, nil
	}

	sentTxo, ok := s.SentTxs[string(txHash)]
	if ok {
		sentTxoBytes, err := types.Cbor.Marshal(sentTxo)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal sent transaction order: %w", err)
		}
		txr := &types.TransactionRecord{
			Version:          1,
			TransactionOrder: sentTxoBytes,
			ServerMetadata: &types.ServerMetadata{
				SuccessIndicator: 1,
				ActualFee:        1,
			},
		}
		txp := &types.TxProof{Version: 1}
		txRecordProof := &types.TxRecordProof{
			TxRecord: txr,
			TxProof:  txp,
		}
		txRecordProofCBOR, err := types.Cbor.Marshal(txRecordProof)
		if err != nil {
			return nil, err
		}
		return &sdktypes.TransactionRecordAndProof{
			TxRecordProof: txRecordProofCBOR,
		}, nil
	}
	return nil, nil
}

func (s *StateServiceMock) GetBlock(ctx context.Context, roundNumber hex.Uint64) (hex.Bytes, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Block, nil
}

func (s *StateServiceMock) Reset() {
	s.RoundNumber = 0
	s.Units = nil
	s.OwnerUnitIDs = nil
	s.TxProofs = nil
	s.Err = nil
	s.Block = nil
	s.GetUnitCalls = 0
}
