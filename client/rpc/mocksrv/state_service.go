package mocksrv

import (
	"context"
	"crypto"

	abrpc "github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

type (
	StateServiceMock struct {
		RoundNumber  uint64
		Units        map[string]*abrpc.Unit[any]
		OwnerUnitIDs map[string][]types.UnitID
		TxProofs     map[string]*abrpc.TransactionRecordAndProof
		Block        types.Bytes
		SentTxs      map[string]*types.TransactionOrder
		Err          error
	}

	Options struct {
		Err          error
		RoundNumber  uint64
		TxProofs     map[string]*abrpc.TransactionRecordAndProof
		Units        map[string]*abrpc.Unit[any]
		OwnerUnits   map[string][]types.UnitID
		InfoResponse *abrpc.NodeInfoResponse
	}

	Option func(*Options)
)

func NewStateServiceMock(opts ...Option) *StateServiceMock {
	options := &Options{
		TxProofs:   map[string]*abrpc.TransactionRecordAndProof{},
		Units:      map[string]*abrpc.Unit[any]{},
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

func WithOwnerUnit(unit *abrpc.Unit[any]) Option {
	return func(o *Options) {
		o.Units[string(unit.UnitID)] = unit
		o.OwnerUnits[string(unit.OwnerPredicate)] = append(o.OwnerUnits[string(unit.OwnerPredicate)], unit.UnitID)
	}
}

func WithUnit(unit *abrpc.Unit[any]) Option {
	return func(o *Options) {
		o.Units[string(unit.UnitID)] = unit
	}
}

func WithUnits(units ...*abrpc.Unit[any]) Option {
	return func(o *Options) {
		for _, unit := range units {
			o.Units[string(unit.UnitID)] = unit
		}
	}
}

func WithTxProof(txHash []byte, txProof *abrpc.TransactionRecordAndProof) Option {
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

func (s *StateServiceMock) GetRoundNumber(ctx context.Context) (uint64, error) {
	if s.Err != nil {
		return 0, s.Err
	}
	return s.RoundNumber, nil
}

func (s *StateServiceMock) GetUnit(unitID types.UnitID, includeStateProof bool) (*abrpc.Unit[any], error) {
	if s.Err != nil {
		return nil, s.Err
	}
	u, ok := s.Units[string(unitID)]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (s *StateServiceMock) GetUnitsByOwnerID(ownerID types.Bytes) ([]types.UnitID, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.OwnerUnitIDs[string(ownerID)], nil
}

func (s *StateServiceMock) SendTransaction(ctx context.Context, tx types.Bytes) (types.Bytes, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	var txo *types.TransactionOrder
	if err := cbor.Unmarshal(tx, &txo); err != nil {
		return nil, err
	}
	txHash := txo.Hash(crypto.SHA256)
	s.SentTxs[string(txHash)] = txo
	return txHash, nil
}

func (s *StateServiceMock) GetTransactionProof(ctx context.Context, txHash types.Bytes) (*abrpc.TransactionRecordAndProof, error) {
	if s.Err != nil {
		return nil, s.Err
	}

	txProof, ok := s.TxProofs[string(txHash)]
	if ok {
		return txProof, nil
	}

	sentTxo, ok := s.SentTxs[string(txHash)]
	if ok {
		txrBytes, err := cbor.Marshal(&types.TransactionRecord{TransactionOrder: sentTxo, ServerMetadata: &types.ServerMetadata{ActualFee: 1}})
		if err != nil {
			return nil, err
		}
		txProofBytes, err := cbor.Marshal(&types.TxProof{})
		if err != nil {
			return nil, err
		}
		return &abrpc.TransactionRecordAndProof{
			TxRecord: txrBytes,
			TxProof:  txProofBytes,
		}, nil
	}
	return nil, nil
}

func (s *StateServiceMock) GetBlock(ctx context.Context, roundNumber uint64) (types.Bytes, error) {
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
}
