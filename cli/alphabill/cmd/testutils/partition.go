package testutils

import (
	"crypto"
	"log/slog"
	"net"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/rpc/alphabill"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	testblock "github.com/alphabill-org/alphabill-wallet/internal/testutils/block"
	testlogger "github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

func CreateMoneyPartition(t *testing.T, genesisConfig *testutil.MoneyGenesisConfig, nodeCount uint8) *testpartition.NodePartition {
	genesisState := testutil.MoneyGenesisState(t, genesisConfig)
	moneyPart, err := testpartition.NewPartition(t, nodeCount, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		genesisState = genesisState.Clone()
		system, err := money.NewTxSystem(
			testobserve.Default(t),
			money.WithSystemIdentifier(money.DefaultSystemIdentifier),
			money.WithHashAlgorithm(crypto.SHA256),
			money.WithSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
				{
					SystemIdentifier: money.DefaultSystemIdentifier,
					T2Timeout:        testblock.DefaultT2Timeout,
					FeeCreditBill: &genesis.FeeCreditBill{
						UnitID:         money.NewBillID(nil, []byte{2}),
						OwnerPredicate: templates.AlwaysTrueBytes(),
					},
				},
			}),
			money.WithTrustBase(tb),
			money.WithState(genesisState),
		)
		require.NoError(t, err)
		return system
	}, money.DefaultSystemIdentifier, genesisState)
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

func StartPartitionRPCServers(t *testing.T, partition *testpartition.NodePartition) {
	for _, n := range partition.Nodes {
		n.AddrGRPC = startRPCServer(t, n.Node, testlogger.NOP())
	}
	// wait for partition servers to start
	for _, n := range partition.Nodes {
		require.Eventually(t, func() bool {
			_, err := n.LatestBlockNumber()
			return err == nil
		}, test.WaitDuration, test.WaitTick)
	}
}

func startRPCServer(t *testing.T, node *partition.Node, log *slog.Logger) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer, err := initRPCServer(node, &grpcServerConfiguration{
		address:               listener.Addr().String(),
		maxGetBlocksBatchSize: defaultMaxGetBlocksBatchSize,
		maxRecvMsgSize:        defaultMaxRecvMsgSize,
		maxSendMsgSize:        defaultMaxSendMsgSize,
	}, testobserve.Default(t), log)
	require.NoError(t, err)

	t.Cleanup(func() {
		grpcServer.GracefulStop()
	})

	go func() {
		require.NoError(t, grpcServer.Serve(listener), "gRPC server exited with error")
	}()

	return listener.Addr().String()
}

func initRPCServer(node *partition.Node, cfg *grpcServerConfiguration, obs partition.Observability, log *slog.Logger) (*grpc.Server, error) {
	grpcServer := grpc.NewServer(
		grpc.MaxSendMsgSize(cfg.maxSendMsgSize),
		grpc.MaxRecvMsgSize(cfg.maxRecvMsgSize),
		grpc.KeepaliveParams(cfg.grpcKeepAliveServerParameters()),
		grpc.UnaryInterceptor(rpc.InstrumentMetricsUnaryServerInterceptor(obs.Meter(rpc.MetricsScopeGRPCAPI), log)),
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(obs.TracerProvider()))),
	)
	grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())

	rpcServer, err := rpc.NewGRPCServer(node, obs, rpc.WithMaxGetBlocksBatchSize(cfg.maxGetBlocksBatchSize))
	if err != nil {
		return nil, err
	}

	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	return grpcServer, nil
}

func CreateTokensPartition(t *testing.T) *testpartition.NodePartition {
	tokensState := state.NewEmptyState()
	network, err := testpartition.NewPartition(t, 1,
		func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
			tokensState = tokensState.Clone()
			system, err := tokens.NewTxSystem(
				testobserve.Default(t),
				tokens.WithState(tokensState),
				tokens.WithTrustBase(tb),
			)
			require.NoError(t, err)
			return system
		}, tokens.DefaultSystemIdentifier, tokensState,
	)
	require.NoError(t, err)
	return network
}
