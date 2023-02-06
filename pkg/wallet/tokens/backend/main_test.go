package twb

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
)

func Test_Run(t *testing.T) {
	t.Parallel()

	t.Run("failure to get storage", func(t *testing.T) {
		cfg := &mockCfg{} // no db cfg assigned, will cause error
		err := Run(context.Background(), cfg)
		require.EqualError(t, err, `failed to get storage: neither db file name nor mock is assigned`)
	})

	t.Run("failure to get starting block number from storage", func(t *testing.T) {
		expErr := fmt.Errorf("can't get  block number")
		cfg := &mockCfg{
			db: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 0, expErr },
			},
			abc: &mockABClient{},
		}
		require.NoError(t, cfg.initListener())

		err := Run(context.Background(), cfg)
		require.ErrorIs(t, err, expErr)
	})

	t.Run("failure to fetch new blocks from AB", func(t *testing.T) {
		expErr := fmt.Errorf("AB doesn't return blocks right now")
		cfg := &mockCfg{
			dbFile: filepath.Join(t.TempDir(), "tokens.db"),
			abc: &mockABClient{
				getBlocks: func(blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
					return nil, expErr
				},
			},
		}
		require.NoError(t, cfg.initListener())

		err := Run(context.Background(), cfg)
		require.ErrorIs(t, err, expErr)
	})

	t.Run("cancelling ctx stops the backend", func(t *testing.T) {
		syncing := make(chan struct{})
		cfg := &mockCfg{
			dbFile: filepath.Join(t.TempDir(), "tokens.db"),
			abc: &mockABClient{
				getBlocks: func(blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
					select {
					case syncing <- struct{}{}:
					default:
					}
					// signal "no new blocks" so sync should sit idle
					return &alphabill.GetBlocksResponse{MaxBlockNumber: blockNumber}, nil
				},
			},
		}
		require.NoError(t, cfg.initListener())

		ctx, cancel := context.WithCancel(context.Background())
		srvErr := make(chan error, 1)
		go func() {
			srvErr <- Run(ctx, cfg)
		}()

		select {
		case <-syncing:
		case <-time.After(time.Second):
			t.Error("backend didn't start syncing within timeout")
		}

		// stop the backend
		cancel()
		select {
		case <-time.After(time.Second):
			t.Error("Run didn't return within timeout")
		case err := <-srvErr:
			require.ErrorIs(t, err, context.Canceled)
		}
	})
}

func Test_Run_API(t *testing.T) {
	t.Parallel()

	syncing := make(chan *txsystem.Transaction)
	// only AB backend is mocked, rest is "real"
	cfg := &mockCfg{
		errLog: func(a ...any) { t.Errorf("ERROR LOG: %v", a) },
		dbFile: filepath.Join(t.TempDir(), "tokens.db"),
		abc: &mockABClient{
			sendTransaction: func(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
				syncing <- tx
				return &txsystem.TransactionResponse{Ok: true}, nil
			},
			getBlocks: func(blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
				select {
				case tx := <-syncing:
					return &alphabill.GetBlocksResponse{
						MaxBlockNumber: blockNumber,
						Blocks: []*block.Block{{
							SystemIdentifier: tx.SystemId,
							BlockNumber:      blockNumber,
							Transactions:     []*txsystem.Transaction{tx},
						}},
					}, nil
				default:
					// signal "no new blocks"
					return &alphabill.GetBlocksResponse{MaxBlockNumber: blockNumber}, nil
				}
			},
		},
	}
	require.NoError(t, cfg.initListener())

	doGet := func(path string, code int, data any) error {
		rsp, err := http.Get(cfg.HttpURL(path))
		if err != nil {
			return fmt.Errorf("request to %q failed: %w", path, err)
		}
		return decodeResponse(t, rsp, code, data)
	}

	getRoundNumber := func() uint64 {
		t.Helper()
		var rn RoundNumberResponse
		require.NoError(t, doGet("/round-number", http.StatusOK, &rn))
		return rn.RoundNumber
	}

	waitForRoundNumber := func(num uint64, timeout time.Duration) {
		t.Helper()
		for st := time.Now(); ; {
			if getRoundNumber() == num {
				break
			}
			if et := time.Since(st); et > timeout {
				t.Fatalf("%s has elapsed but still don't see round-number %d", et, num)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// launch the backend
	ctx, cancel := context.WithCancel(context.Background())
	srvDone := make(chan struct{})
	go func() {
		close(srvDone)
		if err := Run(ctx, cfg); err == nil || !errors.Is(err, context.Canceled) {
			t.Errorf("Run exited with unexpected error: %v", err)
		}
	}()

	require.EqualValues(t, 0, getRoundNumber(), "expected that system starts with round-number 0")

	// trigger block sync from (mocked) AB with an CreateNonFungibleTokenType tx
	createNTFTypeTx := randomTx(t, &tokens.CreateNonFungibleTokenTypeAttributes{Symbol: "test"})
	select {
	case syncing <- createNTFTypeTx:
	case <-time.After(2 * time.Second):
		t.Error("backend didn't start syncing within timeout")
	}

	// syncing with mocked AB backend should have us now on round-number 1
	waitForRoundNumber(1, 1000*time.Millisecond)

	// we synced NTF token type from backend, check that it is returned:
	// first convert the txsystem.Transaction to the type we have in indexing backend...
	txs, err := tokens.New()
	if err != nil {
		t.Errorf("failed to create token tx system: %v", err)
	}
	gtx, err := txs.ConvertTx(createNTFTypeTx)
	if err != nil {
		t.Fatalf("failed to convert tx: %v", err)
	}
	tx := gtx.(tokens.CreateNonFungibleTokenType)
	cnfttt := &TokenUnitType{
		Kind:                     NonFungible,
		ID:                       util.Uint256ToBytes(gtx.UnitID()),
		ParentTypeID:             tx.ParentTypeID(),
		Symbol:                   tx.Symbol(),
		SubTypeCreationPredicate: tx.SubTypeCreationPredicate(),
		TokenCreationPredicate:   tx.TokenCreationPredicate(),
		InvariantPredicate:       tx.InvariantPredicate(),
		NftDataUpdatePredicate:   tx.DataUpdatePredicate(),
		TxHash:                   gtx.Hash(crypto.SHA256),
	}
	//...and check do we get it back via API
	// get all kind of types
	var typesData []*TokenUnitType
	require.NoError(t, doGet("/kinds/all/types", http.StatusOK, &typesData))
	require.ElementsMatch(t, typesData, []*TokenUnitType{cnfttt})
	// there shouldn't be any fungible token types
	typesData = nil
	require.NoError(t, doGet("/kinds/fungible/types", http.StatusOK, &typesData))
	require.Empty(t, typesData)
	// ask for nft types only
	typesData = nil
	require.NoError(t, doGet("/kinds/nft/types", http.StatusOK, &typesData))
	require.ElementsMatch(t, typesData, []*TokenUnitType{cnfttt})

	// post an tx to mint NFT with the existing type
	ownerID := test.RandomBytes(33)
	vphex := hexutil.Encode(ownerID)
	message, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(&txsystem.Transactions{
		Transactions: []*txsystem.Transaction{randomTx(t, &tokens.MintNonFungibleTokenAttributes{Bearer: ownerID, NftType: createNTFTypeTx.UnitId})},
	})
	require.NoError(t, err)
	require.NotEmpty(t, message)

	rsp, err := http.Post(cfg.HttpURL("/transactions/"+vphex), "", bytes.NewBuffer(message))
	require.NoError(t, err)
	require.NotNil(t, rsp)
	data := map[string]string{}
	require.NoError(t, decodeResponse(t, rsp, http.StatusAccepted, &data))
	require.Empty(t, data)

	// syncing with mocked AB backend should have us now on round-number 2
	waitForRoundNumber(2, 1000*time.Millisecond)

	// read back the token we minted
	var tokens []*TokenUnit
	require.NoError(t, doGet("/kinds/nft/owners/"+vphex+"/tokens", http.StatusOK, &tokens))
	require.Len(t, tokens, 1, "expected that one token is found")
	// should get no fungible tokens
	require.NoError(t, doGet("/kinds/fungible/owners/"+vphex+"/tokens", http.StatusOK, &tokens))
	require.Empty(t, tokens, "expected no fungible tokens to be found")

	// stop the backend
	cancel()
	select {
	case <-time.After(time.Second):
		t.Error("Run didn't return within timeout")
	case <-srvDone:
	}
}
