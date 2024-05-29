package testutils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const defaultAlphabillDockerImage string = "ghcr.io/alphabill-org/alphabill:cf4ff7151d7a7ebba65903b7d827b0740fc878a4"

type (
	AlphabillNetwork struct {
		MoneyRpcUrl         string
		TokensRpcUrl        string
		OrchestrationRpcUrl string

		ctx                 context.Context
		genesis             []byte
		dockerNetwork       string
		bootstrapNode       string
	}

	Wallet struct {
		Homedir string
		PubKeys [][]byte
	}

	StdoutLogConsumer struct{}
)

var nodeWaitStrategy = wait.ForHTTP("/rpc").
	WithPort("8001").
	WithHeaders(map[string]string{"Content-Type": "application/json"}).
	WithBody(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"state_getRoundNumber"}`)).
	WithStartupTimeout(5*time.Second)


func (lc *StdoutLogConsumer) Accept(l tc.Log) {
    fmt.Print(string(l.Content))
}

func dockerImage() string {
	image := os.Getenv("AB_TEST_DOCKERIMAGE")
	if image == "" {
		return defaultAlphabillDockerImage
	}
	return image
}

// SetupNetworkWithWallets sets up the Alphabill network and creates two wallets with two keys in both of them.
// Starts money partition, and optionally tokens and orchestration partitions, with rpc servers up and running.
// The owner of the initial bill is set to the first key of the first wallet.
// Returns the created wallets and a reference to the Alphabill network.
func SetupNetworkWithWallets(t *testing.T, withTokensNode, withOrchestrationNode bool) ([]*Wallet, *AlphabillNetwork) {
	ctx := context.Background()
	dockerNetwork, err := network.New(ctx, network.WithCheckDuplicate())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, dockerNetwork.Remove(ctx))
	})

	abNet := &AlphabillNetwork{
		ctx:           ctx,
		dockerNetwork: dockerNetwork.Name,
	}

	wallets := setupWallets(t, 2, 2)
	ownerPredicate := templates.NewP2pkh256BytesFromKey(wallets[0].PubKeys[0])

	abNet.createGenesis(t, ownerPredicate)
	abNet.startRootNode(t)
	abNet.startMoneyNode(t)

	if withTokensNode {
		abNet.startTokensNode(t)
	}
	if withOrchestrationNode {
		abNet.startOrchestrationNode(t)
	}

	return wallets, abNet
}

func setupWallets(t *testing.T, walletCount, keyCount int) []*Wallet{
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

func (n *AlphabillNetwork) createGenesis(t *testing.T, ownerPredicate []byte) {
	cr := tc.ContainerRequest{
		Image: dockerImage(),
		WaitingFor: wait.ForExit().WithExitTimeout(5*time.Second),
		LogConsumerCfg: &tc.LogConsumerConfig{
			Consumers: []tc.LogConsumer{&StdoutLogConsumer{}},
		},
		Files: []tc.ContainerFile{{
			HostFilePath:      "./testdata/genesis.sh",
			ContainerFilePath: "/app/genesis.sh",
			FileMode:          0o755,
		}},
		Entrypoint: []string{"genesis.sh"},
		Cmd: []string{
			fmt.Sprintf("%X", ownerPredicate),
		},
	}
	gcr := tc.GenericContainerRequest{
		ContainerRequest: cr,
		Started:          true,
	}
	gc, err := tc.GenericContainer(n.ctx, gcr)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, gc.Terminate(n.ctx))
	})

	genesisReader, err := gc.CopyFileFromContainer(n.ctx, "/app/genesis.tar")
	require.NoError(t, err)

	genesis, err := io.ReadAll(genesisReader)
	require.NoError(t, err)
	n.genesis = genesis
}

func (n *AlphabillNetwork) startRootNode(t *testing.T) {
	cr := tc.ContainerRequest{
		Image: dockerImage(),
		WaitingFor: wait.ForLog("Starting root node").WithStartupTimeout(5*time.Second),
		LogConsumerCfg: &tc.LogConsumerConfig{
			Consumers: []tc.LogConsumer{&StdoutLogConsumer{}},
		},
		Entrypoint: []string{
			"/home/nonroot/alphabill.sh",
		},
		Files: []tc.ContainerFile{{
			HostFilePath: "./testdata/alphabill.sh",
			ContainerFilePath: "/home/nonroot/alphabill.sh",
			FileMode: 0o755,
		}, {
			Reader: bytes.NewReader(n.genesis),
			ContainerFilePath: "/home/nonroot/genesis.tar",
			FileMode: 0o755,
		}},
		Cmd: []string{
			"root",
			"--home", "/home/nonroot/root1",
			"--address", "/ip4/0.0.0.0/tcp/8000",
			"--log-file", "stdout",
			"--log-level", "info",
			"--log-format", "text",
			"--trust-base-file", "/home/nonroot/root-trust-base.json",
		},
		Networks: []string{n.dockerNetwork},
	}

	gcr := tc.GenericContainerRequest{
		ContainerRequest: cr,
		Started:          true,
	}
	gc, err := tc.GenericContainer(n.ctx, gcr)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, gc.Terminate(n.ctx))
	})

	ip, err := gc.ContainerIP(n.ctx)
	require.NoError(t, err)

	_, r, err := gc.Exec(n.ctx, []string{
		"alphabill", "identifier", "--key-file", "/home/nonroot/root1/rootchain/keys.json",
	}, exec.Multiplexed())
	require.NoError(t, err)

	id, err := io.ReadAll(r)
	require.NoError(t, err)
	n.bootstrapNode = fmt.Sprintf("%s@/ip4/%s/tcp/8000", strings.TrimSpace(string(id)), ip)
}

func (n *AlphabillNetwork) startMoneyNode(t *testing.T) {
	cr := tc.ContainerRequest{
		Image: dockerImage(),
		WaitingFor: nodeWaitStrategy,
		LogConsumerCfg: &tc.LogConsumerConfig{
			Consumers: []tc.LogConsumer{&StdoutLogConsumer{}},
		},
		Entrypoint: []string{
			"/home/nonroot/alphabill.sh",
		},
		Files: []tc.ContainerFile{{
			HostFilePath: "./testdata/alphabill.sh",
			ContainerFilePath: "/home/nonroot/alphabill.sh",
			FileMode: 0o755,
		}, {
			Reader: bytes.NewReader(n.genesis),
			ContainerFilePath: "/home/nonroot/genesis.tar",
			FileMode: 0o755,
		}},

		Cmd: []string{
			"money",
			"--home", "/home/nonroot/money1",
			"--address", "/ip4/0.0.0.0/tcp/8000",
			"--rpc-server-address", "0.0.0.0:8001",
			"--log-file", "stdout",
			"--log-level", "info",
			"--log-format", "text",
			"--genesis",  "/home/nonroot/root1/rootchain/partition-genesis-1.json",
			"--key-file", "/home/nonroot/money1/money/keys.json",
			"--state",    "/home/nonroot/money1/money/node-genesis-state.cbor",
			"--db",       "/home/nonroot/money1/money/blocks.db",
			"--tx-db",    "/home/nonroot/money1/money/tx.db",
			"--bootnodes", n.bootstrapNode,
			"--trust-base-file", "/home/nonroot/root-trust-base.json",
		},
		Networks: []string{n.dockerNetwork},
		ExposedPorts: []string{"8001"},
	}

	gcr := tc.GenericContainerRequest{
		ContainerRequest: cr,
		Started:          true,
	}
	gc, err := tc.GenericContainer(n.ctx, gcr)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, gc.Terminate(n.ctx))
	})

	rpcPort, err := gc.MappedPort(n.ctx, "8001")
	require.NoError(t, err)
	n.MoneyRpcUrl = fmt.Sprintf("http://127.0.0.1:%s/rpc", rpcPort.Port())
}

func (n *AlphabillNetwork) startTokensNode(t *testing.T) {
	cr := tc.ContainerRequest{
		Image: dockerImage(),
		WaitingFor: nodeWaitStrategy,
		LogConsumerCfg: &tc.LogConsumerConfig{
			Consumers: []tc.LogConsumer{&StdoutLogConsumer{}},
		},
		Entrypoint: []string{
			"/home/nonroot/alphabill.sh",
		},
		Files: []tc.ContainerFile{{
			HostFilePath: "./testdata/alphabill.sh",
			ContainerFilePath: "/home/nonroot/alphabill.sh",
			FileMode: 0o755,
		}, {
			Reader: bytes.NewReader(n.genesis),
			ContainerFilePath: "/home/nonroot/genesis.tar",
			FileMode: 0o755,
		}},

		Cmd: []string{
			"tokens",
			"--home", "/home/nonroot/tokens1",
			"--address", "/ip4/0.0.0.0/tcp/8000",
			"--rpc-server-address", "0.0.0.0:8001",
			"--log-file", "stdout",
			"--log-level", "info",
			"--log-format", "text",
			"--genesis",  "/home/nonroot/root1/rootchain/partition-genesis-2.json",
			"--key-file", "/home/nonroot/tokens1/tokens/keys.json",
			"--state",    "/home/nonroot/tokens1/tokens/node-genesis-state.cbor",
			"--db",       "/home/nonroot/tokens1/tokens/blocks.db",
			"--tx-db",    "/home/nonroot/tokens1/tokens/tx.db",
			"--bootnodes", n.bootstrapNode,
			"--trust-base-file", "/home/nonroot/root-trust-base.json",
		},
		Networks: []string{n.dockerNetwork},
		ExposedPorts: []string{"8001"},
	}

	gcr := tc.GenericContainerRequest{
		ContainerRequest: cr,
		Started:          true,
	}
	gc, err := tc.GenericContainer(n.ctx, gcr)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, gc.Terminate(n.ctx))
	})

	rpcPort, err := gc.MappedPort(n.ctx, "8001")
	require.NoError(t, err)

	n.TokensRpcUrl = fmt.Sprintf("http://127.0.0.1:%s/rpc", rpcPort.Port())
}

func (n *AlphabillNetwork) startOrchestrationNode(t *testing.T) {
	cr := tc.ContainerRequest{
		Image: dockerImage(),
		WaitingFor: nodeWaitStrategy,
		LogConsumerCfg: &tc.LogConsumerConfig{
			Consumers: []tc.LogConsumer{&StdoutLogConsumer{}},
		},
		Entrypoint: []string{
			"/home/nonroot/alphabill.sh",
		},
		Files: []tc.ContainerFile{{
			HostFilePath: "./testdata/alphabill.sh",
			ContainerFilePath: "/home/nonroot/alphabill.sh",
			FileMode: 0o755,
		}, {
			Reader: bytes.NewReader(n.genesis),
			ContainerFilePath: "/home/nonroot/genesis.tar",
			FileMode: 0o755,
		}},

		Cmd: []string{
			"orchestration",
			"--home", "/home/nonroot/orchestration1",
			"--address", "/ip4/0.0.0.0/tcp/8000",
			"--rpc-server-address", "0.0.0.0:8001",
			"--log-file", "stdout",
			"--log-level", "info",
			"--log-format", "text",
			"--genesis",  "/home/nonroot/root1/rootchain/partition-genesis-4.json",
			"--key-file", "/home/nonroot/orchestration1/orchestration/keys.json",
			"--state",    "/home/nonroot/orchestration1/orchestration/node-genesis-state.cbor",
			"--db",       "/home/nonroot/orchestration1/orchestration/blocks.db",
			"--tx-db",    "/home/nonroot/orchestration1/orchestration/tx.db",
			"--bootnodes", n.bootstrapNode,
			"--trust-base-file", "/home/nonroot/root-trust-base.json",
		},
		Networks: []string{n.dockerNetwork},
		ExposedPorts: []string{"8001"},
	}

	gcr := tc.GenericContainerRequest{
		ContainerRequest: cr,
		Started:          true,
	}
	gc, err := tc.GenericContainer(n.ctx, gcr)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, gc.Terminate(n.ctx))
	})

	rpcPort, err := gc.MappedPort(n.ctx, "8001")
	require.NoError(t, err)

	n.OrchestrationRpcUrl = fmt.Sprintf("http://127.0.0.1:%s/rpc", rpcPort.Port())
}
