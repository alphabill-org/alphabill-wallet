package client

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
	"github.com/holiman/uint256"
)

type (
	evmPartitionClient struct {
		*partitionClient
	}

	// TODO: these structs are also defined in alphabill/txsystem/evm/statedb,
	// should be moved to alphabill-go-base
	stateObject struct {
		Account   *account
		AlphaBill *alphaBillLink
	}

	account struct {
		Balance *uint256.Int
	}

	alphaBillLink struct {
		Counter        uint64
		MinLifetime    uint64
		OwnerPredicate []byte
	}
)

// NewEvmPartitionClient creates an evm partition client for the given RPC URL.
func NewEvmPartitionClient(ctx context.Context, rpcUrl string) (sdktypes.PartitionClient, error) {
	partitionClient, err := newPartitionClient(ctx, rpcUrl, evm.PartitionTypeID)
	if err != nil {
		return nil, err
	}

	return &evmPartitionClient{
		partitionClient: partitionClient,
	}, nil
}

// GetFeeCreditRecordByOwnerID finds the first fee credit record in evm partition for the given owner ID,
// returns nil if fee credit record does not exist.
func (c *evmPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch units: %w", err)
	}
	if len(unitIDs) == 0 {
		return nil, nil
	}
	var u *sdktypes.Unit[stateObject]
	if err := c.RpcClient.CallContext(ctx, &u, "state_getUnit", unitIDs[0], false); err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	stateObj := u.Data
	fcr := &fc.FeeCreditRecord{
		Balance:     weiToAlpha(stateObj.Account.Balance),
		Counter:     stateObj.AlphaBill.Counter,
		MinLifetime: stateObj.AlphaBill.MinLifetime,
	}
	counterCopy := fcr.Counter
	return &sdktypes.FeeCreditRecord{
		NetworkID:   u.NetworkID,
		PartitionID: u.PartitionID,
		ID:          u.UnitID,
		Balance:     fcr.Balance,
		Counter:     &counterCopy,
		MinLifetime: fcr.MinLifetime,
		StateLockTx: u.StateLockTx,
	}, nil
}

// TODO: copied from AB repo, move to go-base?
var alpha2Wei = new(uint256.Int).Exp(uint256.NewInt(10), uint256.NewInt(10))
var alpha2WeiRoundCorrector = new(uint256.Int).Div(alpha2Wei, uint256.NewInt(2))

// weiToAlpha - converts from wei to alpha, rounding half up.
// 1 wei = wei * 10^10 / 10^18
func weiToAlpha(wei *uint256.Int) uint64 {
	return new(uint256.Int).Div(new(uint256.Int).Add(wei, alpha2WeiRoundCorrector), alpha2Wei).Uint64()
}

func (c *evmPartitionClient) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*types.TxRecordProof, error) {
	sub, err := txsubmitter.New(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx submission: %w", err)
	}
	txBatch := sub.ToBatch(c, log)

	if err := txBatch.SendTx(ctx, true); err != nil {
		return nil, err
	}
	return txBatch.Submissions()[0].Proof, nil
}

func (c *evmPartitionClient) Close() {
	c.AdminAPIClient.Close()
	c.StateAPIClient.Close()
}
