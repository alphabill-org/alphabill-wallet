package testutils

import (
	"context"
	"crypto"
	"net"
	"net/http"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	sdkmoney "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	sdktokens "github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	sdkorchestration "github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/partition"
	abrpc "github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/txsystem/orchestration"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

const DefaultT2Timeout = uint32(2500)

type Wallet struct {
	Homedir string
	PubKeys [][]byte
}

func createMoneyPartition(t *testing.T, genesisConfig *testutil.MoneyGenesisConfig, nodeCount uint8) *NodePartition {
	genesisState := testutil.MoneyGenesisState(t, genesisConfig)
	moneyPart, err := newPartition(t, "money node", nodeCount, func(tb types.RootTrustBase) txsystem.TransactionSystem {
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

func CreateTokensPartition(t *testing.T) *NodePartition {
	tokensState := state.NewEmptyState()
	network, err := newPartition(t, "tokens node", 1,
		func(tb types.RootTrustBase) txsystem.TransactionSystem {
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

func CreateOrchestrationPartition(t *testing.T, ownerPredicate types.PredicateBytes) *NodePartition {
	s := state.NewEmptyState()
	network, err := newPartition(t,	"orchestration node", 1,
		func(tb types.RootTrustBase) txsystem.TransactionSystem {
			s = s.Clone()
			txSystem, err := orchestration.NewTxSystem(
				testobserve.Default(t),
				orchestration.WithState(s),
				orchestration.WithTrustBase(tb),
				orchestration.WithOwnerPredicate(ownerPredicate),
			)
			require.NoError(t, err)
			return txSystem
		},
		sdkorchestration.DefaultSystemID,
		s,
	)
	require.NoError(t, err)
	return network
}

func startRpcServers(t *testing.T, partition *NodePartition) {
	for _, n := range partition.Nodes {
		n.AddrRPC = startRpcServer(t, n.Node, partition.SystemName, n.OwnerIndexer)
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
		}, WaitDuration, WaitTick)
	}
}

func startRpcServer(t *testing.T, node *partition.Node, nodeName string, ownerIndexer partition.IndexReader) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	rpcServer, err := initRpcServer(
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

func initRpcServer(node *partition.Node, nodeName string, cfg *abrpc.ServerConfiguration, ownerIndexer partition.IndexReader, obs partition.Observability) (*http.Server, error) {
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

// setupNetwork starts alphabill network.
// Starts money partition, and optionally any other partitions, with rpc servers up and running.
// Returns money node url and reference to the network object.
func SetupNetwork(t *testing.T, initialBillOwner []byte, otherPartitions ...*NodePartition) *AlphabillNetwork {
	genesisConfig := &testutil.MoneyGenesisConfig{
		InitialBillID:      DefaultInitialBillID,
		InitialBillValue:   1e18,
		InitialBillOwner:   templates.NewP2pkh256BytesFromKey(initialBillOwner),
		DCMoneySupplyValue: 10000,
	}

	moneyPartition := createMoneyPartition(t, genesisConfig, 1)
	partitions := append([]*NodePartition{moneyPartition}, otherPartitions...)

	network, err := newAlphabillNetwork(partitions)
	require.NoError(t, err)
	require.NoError(t, network.start(t))
	t.Cleanup(func() { network.waitClose(t) })

	for _, partition := range partitions {
		startRpcServers(t, partition)
	}
	return network
}

func SetupWallets(t *testing.T, walletCount, keyCount int) []*Wallet{
	var wallets []*Wallet
	for i := 0; i < walletCount; i++ {
		am, home := CreateNewWallet(t)
		defer am.Close()

		pubKey, err := am.GetPublicKey(0)
		require.NoError(t, err)

		keys := [][]byte{pubKey}
		for i := 1; i < keyCount; i++ {
			_, pubKey, err := am.AddAccount()
			require.NoError(t, err)
			keys = append(keys, pubKey)
		}

		wallets = append(wallets, &Wallet{home, keys})
	}
	return wallets
}

// SetupNetworkWithWallets sets up the Alphabill network and also two wallets with two keys in both of them.
// Starts money partition, and optionally any other partitions, with rpc servers up and running.
// The owner of the initial bill is set to the first key of the first wallet.
// Returns the created wallets and a reference to the Alphabill network network.
func SetupNetworkWithWallets(t *testing.T, otherPartitions ...*NodePartition) ([]*Wallet, *AlphabillNetwork) {
	wallets := SetupWallets(t, 2, 2)
	network := SetupNetwork(t, wallets[0].PubKeys[0], otherPartitions...)
	return wallets, network
}
