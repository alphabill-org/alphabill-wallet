package mocksrv

import (
	"context"
	"crypto"
	"net"
	"net/http"
	"testing"

	abrpc "github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/wallet/evm/client"
)

func StartServer(t *testing.T, service *MockService) string {
	server := ethrpc.NewServer()
	t.Cleanup(server.Stop)

	err := server.RegisterName("state", service)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	httpServer := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: server,
	}

	go httpServer.Serve(listener)
	t.Cleanup(func() {
		_ = httpServer.Close()
	})
	return httpServer.Addr
}

type (
	MockService struct {
		RoundNumber  uint64
		Units        map[string]*abrpc.Unit[any]
		OwnerUnitIDs map[string][]types.UnitID
		TxProofs     map[string]*abrpc.TransactionRecordAndProof
		Block        types.Bytes
		SentTxs      map[string]*types.TransactionOrder
		Err          error
	}

	Options struct {
		Err         error
		RoundNumber uint64
		TxProofs    map[string]*abrpc.TransactionRecordAndProof
		Units       map[string]*abrpc.Unit[any]
		OwnerUnits  map[string][]types.UnitID
	}

	Option func(*Options)
)

func NewRpcServerMock(opts ...Option) *MockService {
	options := &Options{
		TxProofs:   map[string]*abrpc.TransactionRecordAndProof{},
		Units:      map[string]*abrpc.Unit[any]{},
		OwnerUnits: map[string][]types.UnitID{},
	}
	for _, option := range opts {
		option(options)
	}
	return &MockService{
		Err:          options.Err,
		RoundNumber:  options.RoundNumber,
		Units:        options.Units,
		OwnerUnitIDs: options.OwnerUnits,
		TxProofs:     options.TxProofs,
		SentTxs:      map[string]*types.TransactionOrder{},
	}
}

func WithOwnerBill(unit *abrpc.Unit[any]) Option {
	return func(o *Options) {
		o.Units[string(unit.UnitID)] = unit
		o.OwnerUnits[string(unit.OwnerPredicate)] = append(o.OwnerUnits[string(unit.OwnerPredicate)], unit.UnitID)
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

func (s *MockService) GetRoundNumber(ctx context.Context) (uint64, error) {
	if s.Err != nil {
		return 0, s.Err
	}
	return s.RoundNumber, nil
}

func (s *MockService) GetUnit(unitID types.UnitID, includeStateProof bool) (*abrpc.Unit[any], error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Units[string(unitID)], nil
}

func (s *MockService) GetUnitsByOwnerID(ownerID types.Bytes) ([]types.UnitID, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.OwnerUnitIDs[string(ownerID)], nil
}

func (s *MockService) SendTransaction(ctx context.Context, tx types.Bytes) (types.Bytes, error) {
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

func (s *MockService) GetTransactionProof(ctx context.Context, txHash types.Bytes) (*abrpc.TransactionRecordAndProof, error) {
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
	return nil, client.ErrNotFound
}

func (s *MockService) GetBlock(ctx context.Context, roundNumber uint64) (types.Bytes, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Block, nil
}

func (s *MockService) Reset() {
	s.RoundNumber = 0
	s.Units = nil
	s.OwnerUnitIDs = nil
	s.TxProofs = nil
	s.Err = nil
}
