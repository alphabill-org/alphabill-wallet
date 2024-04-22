package testutils

import (
	"context"
	"crypto"
	"net"
	"net/http"
	"testing"

	sdkcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	sdkmoney "github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	sdktokens "github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"

	"github.com/alphabill-org/alphabill/partition"
	abrpc "github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

const DefaultT2Timeout = uint32(2500)

func CreateMoneyPartition(t *testing.T, genesisConfig *testutil.MoneyGenesisConfig, nodeCount uint8) *testpartition.NodePartition {
	genesisState := testutil.MoneyGenesisState(t, genesisConfig)
	moneyPart, err := testpartition.NewPartition(t, "money node", nodeCount, func(tb map[string]sdkcrypto.Verifier) txsystem.TransactionSystem {
		genesisState = genesisState.Clone()
		system, err := money.NewTxSystem(
			testobserve.Default(t),
			money.WithSystemIdentifier(sdkmoney.DefaultSystemID),
			money.WithHashAlgorithm(crypto.SHA256),
			money.WithSystemDescriptionRecords([]*types.SystemDescriptionRecord{
				{
					SystemIdentifier: sdkmoney.DefaultSystemID,
					T2Timeout:        DefaultT2Timeout,
					FeeCreditBill: &types.FeeCreditBill{
						UnitID:         sdkmoney.NewBillID(nil, []byte{2}),
						OwnerPredicate: templates.AlwaysTrueBytes(),
					},
				},
			}),
			money.WithTrustBase(tb),
			money.WithState(genesisState),
		)
		require.NoError(t, err)
		return system
	}, sdkmoney.DefaultSystemID, genesisState)
	require.NoError(t, err)
	return moneyPart
}

func StartAlphabill(t *testing.T, partitions []*testpartition.NodePartition) *testpartition.AlphabillNetwork {
	abNetwork, err := testpartition.NewAlphabillPartition(partitions)
	require.NoError(t, err)
	require.NoError(t, abNetwork.Start(t))
	t.Cleanup(func() { abNetwork.WaitClose(t) })
	return abNetwork
}

func CreateTokensPartition(t *testing.T) *testpartition.NodePartition {
	tokensState := state.NewEmptyState()
	network, err := testpartition.NewPartition(t, "tokens node", 1,
		func(tb map[string]sdkcrypto.Verifier) txsystem.TransactionSystem {
			tokensState = tokensState.Clone()
			system, err := tokens.NewTxSystem(
				testobserve.Default(t),
				tokens.WithState(tokensState),
				tokens.WithTrustBase(tb),
			)
			require.NoError(t, err)
			return system
		}, sdktokens.DefaultSystemID, tokensState,
	)
	require.NoError(t, err)
	return network
}

func StartRpcServers(t *testing.T, partition *testpartition.NodePartition) {
	for _, n := range partition.Nodes {
		n.AddrRPC = StartRpcServer(t, n.Node, partition.SystemName, n.OwnerIndexer)
	}
	// wait for rpc servers to start
	for _, n := range partition.Nodes {
		require.Eventually(t, func() bool {
			rpcClient, err := rpc.DialContext(context.Background(), "http://"+n.AddrRPC+"/rpc")
			if err != nil {
				return false
			}
			defer rpcClient.Close()
			roundNumber, _ := rpcClient.GetRoundNumber(context.Background())
			return roundNumber > 0
		}, test.WaitDuration, test.WaitTick)
	}
}

func StartRpcServer(t *testing.T, node *partition.Node, nodeName string, ownerIndexer partition.IndexReader) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	rpcServer, err := InitRpcServer(
		node,
		nodeName,
		&abrpc.ServerConfiguration{
			Address: listener.Addr().String(),
			// defaults from ab repo
			MaxHeaderBytes:         http.DefaultMaxHeaderBytes,
			MaxBodyBytes:           4194304, // 4MB,
			BatchItemLimit:         1000,
			BatchResponseSizeLimit: 4194304, // 4MB
		},
		ownerIndexer,
		testobserve.Default(t),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = rpcServer.Close()
	})

	go func() {
		err := rpcServer.Serve(listener)
		require.ErrorIs(t, err, http.ErrServerClosed, "rpc server exited with error")
	}()

	return listener.Addr().String()
}

func InitRpcServer(node *partition.Node, nodeName string, cfg *abrpc.ServerConfiguration, ownerIndexer partition.IndexReader, obs partition.Observability) (*http.Server, error) {
	cfg.APIs = []abrpc.API{
		{
			Namespace: "state",
			Service:   abrpc.NewStateAPI(node, ownerIndexer),
		},
		{
			Namespace: "admin",
			Service:   abrpc.NewAdminAPI(node, nodeName, node.Peer(), obs.Logger()),
		},
	}
	httpServer, err := abrpc.NewHTTPServer(cfg, obs)
	if err != nil {
		return nil, err
	}
	return httpServer, nil
}

// SetupNetwork starts alphabill network.
// Starts money partition, and optionally any other partitions, with rpc servers up and running.
// Returns money node url and reference to the network object.
func SetupNetwork(t *testing.T, genesisConfig *testutil.MoneyGenesisConfig, otherPartitions []*testpartition.NodePartition) (string, *testpartition.AlphabillNetwork) {
	moneyPartition := CreateMoneyPartition(t, genesisConfig, 1)
	nodePartitions := []*testpartition.NodePartition{moneyPartition}
	nodePartitions = append(nodePartitions, otherPartitions...)
	abNet := StartAlphabill(t, nodePartitions)

	for _, nodePartition := range nodePartitions {
		StartRpcServers(t, nodePartition)
	}
	return moneyPartition.Nodes[0].AddrRPC, abNet
}
