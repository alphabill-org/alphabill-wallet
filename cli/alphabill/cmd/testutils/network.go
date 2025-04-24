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

const (
	defaultDockerImage   = "ghcr.io/alphabill-org/alphabill:fe5c8db4a4356a7005ed1460569cd0f144f54f5b"
	containerGenesisPath = "/home/nonroot/genesis.tar"
	containerP2pPort     = "8000"
	containerRpcPort     = "8001"
)

var walletMnemonics = []string{
	"ancient unhappy slush month cook fortune capital option sample buzz trip shed",
	"burden resemble casino rebel spend banner lumber diamond word hollow true master",
}

type (
	AlphabillNetwork struct {
		MoneyRpcUrl            string
		TokensRpcUrl           string
		EvmRpcUrl              string
		OrchestrationRpcUrl    string
		EnterpriseTokensRpcUrl string

		ctx           context.Context
		genesis       []byte
		dockerNetwork string
		bootstrapNode string
	}

	AlphabillNetworkOption func(*AlphabillNetwork)

	Wallet struct {
		Homedir string
		PubKeys [][]byte
	}

	StdoutLogConsumer struct{}
)

func (lc *StdoutLogConsumer) Accept(l tc.Log) {
	fmt.Print(string(l.Content))
}

func dockerImage() string {
	image := os.Getenv("AB_TEST_DOCKERIMAGE")
	if image == "" {
		return defaultDockerImage
	}
	return image
}

func WithTokensNode(t *testing.T) AlphabillNetworkOption {
	return func(n *AlphabillNetwork) {
		n.startTokensNode(t)
	}
}

func WithOrchestrationNode(t *testing.T) AlphabillNetworkOption {
	return func(n *AlphabillNetwork) {
		n.startOrchestrationNode(t)
	}
}

func WithEvmNode(t *testing.T) AlphabillNetworkOption {
	return func(n *AlphabillNetwork) {
		n.startEvmNode(t)
	}
}

func WithEnterpriseTokensNode(t *testing.T) AlphabillNetworkOption {
	return func(n *AlphabillNetwork) {
		n.startEnterpriseTokensNode(t)
	}
}

// SetupNetworkWithWallets sets up the Alphabill network and creates two wallets with two keys in both of them.
// Starts money partition, and with given options, tokens, evm and/or orchestration partitions, with rpc servers up and running.
// The owner of the initial bill is set to the first key of the first wallet.
// Returns the created wallets and a reference to the Alphabill network.
func SetupNetworkWithWallets(t *testing.T, opts ...AlphabillNetworkOption) ([]*Wallet, *AlphabillNetwork) {
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

	wallets := SetupWallets(t, 2, 2)
	ownerPredicate := templates.NewP2pkh256BytesFromKey(wallets[0].PubKeys[0])

	abNet.createGenesis(t, ownerPredicate)
	abNet.startRootNode(t)
	abNet.startMoneyNode(t)

	for _, opt := range opts {
		opt(abNet)
	}

	return wallets, abNet
}

func SetupWallets(t *testing.T, walletCount, keyCount int) []*Wallet {
	var wallets []*Wallet
	for i := 0; i < walletCount; i++ {
		am, home := CreateNewWallet(t, getMnemonic(i))
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

func getMnemonic(walletNumber int) string {
	if walletNumber < len(walletMnemonics) {
		return walletMnemonics[walletNumber]
	}
	return ""
}

func (n *AlphabillNetwork) createGenesis(t *testing.T, ownerPredicate []byte) {
	cr := tc.ContainerRequest{
		Image:      dockerImage(),
		WaitingFor: wait.ForExit().WithExitTimeout(10 * time.Second),
		LogConsumerCfg: &tc.LogConsumerConfig{
			Consumers: []tc.LogConsumer{&StdoutLogConsumer{}},
		},
		Files: []tc.ContainerFile{
			{
				HostFilePath:      "./testdata/genesis.sh",
				ContainerFilePath: "/home/nonroot/genesis.sh",
				FileMode:          0o755,
			},
		},
		Entrypoint: []string{"/home/nonroot/genesis.sh"},
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

	genesisReader, err := gc.CopyFileFromContainer(n.ctx, containerGenesisPath)
	require.NoError(t, err)

	genesis, err := io.ReadAll(genesisReader)
	require.NoError(t, err)
	n.genesis = genesis
}

func (n *AlphabillNetwork) startRootNode(t *testing.T) {
	args := []string{
		"root-node", "run",
		"--home", "/home/nonroot/root",
		"--address", "/ip4/0.0.0.0/tcp/"+containerP2pPort,
		"--log-file", "stdout",
		"--log-level", "info",
		"--log-format", "text",
		"--block-rate", "225",
	}
	for i := range 5 {
		args = append(args, "--shard-conf", fmt.Sprintf("/home/nonroot/%d/shard-conf.json", i+1))
	}

	container := n.startNode(t, args...)
	n.bootstrapNode = p2pUrl(t, n.ctx, container, "/home/nonroot/root")
}

func (n *AlphabillNetwork) startMoneyNode(t *testing.T) {
	container := n.startNode(t,
		"shard-node", "run",
		"--home", "/home/nonroot/1",
		"--address", "/ip4/0.0.0.0/tcp/"+containerP2pPort,
		"--rpc-server-address", "0.0.0.0:"+containerRpcPort,
		"--log-file", "stdout",
		"--log-level", "info",
		"--log-format", "text",
		"--bootnodes", n.bootstrapNode,
		"--trust-base", "/home/nonroot/root/trust-base.json",
		"--t1-timeout", "188",
	)
	n.MoneyRpcUrl = rpcUrl(t, n.ctx, container)
}

func (n *AlphabillNetwork) startTokensNode(t *testing.T) {
	container := n.startNode(t,
		"shard-node", "run",
		"--home", "/home/nonroot/2",
		"--address", "/ip4/0.0.0.0/tcp/"+containerP2pPort,
		"--rpc-server-address", "0.0.0.0:"+containerRpcPort,
		"--log-file", "stdout",
		"--log-level", "info",
		"--log-format", "text",
		"--bootnodes", n.bootstrapNode,
		"--trust-base", "/home/nonroot/root/trust-base.json",
		"--t1-timeout", "188",
	)
	n.TokensRpcUrl = rpcUrl(t, n.ctx, container)
}

func (n *AlphabillNetwork) startEvmNode(t *testing.T) {
	container := n.startNode(t,
		"shard-node", "run",
		"--home", "/home/nonroot/3",
		"--address", "/ip4/0.0.0.0/tcp/"+containerP2pPort,
		"--rpc-server-address", "0.0.0.0:"+containerRpcPort,
		"--log-file", "stdout",
		"--log-level", "info",
		"--log-format", "text",
		"--bootnodes", n.bootstrapNode,
		"--trust-base", "/home/nonroot/root/trust-base.json",
		"--t1-timeout", "188",
	)
	n.EvmRpcUrl = rpcUrl(t, n.ctx, container)
}

func (n *AlphabillNetwork) startOrchestrationNode(t *testing.T) {
	container := n.startNode(t,
		"shard-node", "run",
		"--home", "/home/nonroot/4",
		"--address", "/ip4/0.0.0.0/tcp/"+containerP2pPort,
		"--rpc-server-address", "0.0.0.0:"+containerRpcPort,
		"--log-file", "stdout",
		"--log-level", "info",
		"--log-format", "text",
		"--bootnodes", n.bootstrapNode,
		"--trust-base", "/home/nonroot/root/trust-base.json",
		"--t1-timeout", "188",
	)
	n.OrchestrationRpcUrl = rpcUrl(t, n.ctx, container)
}

func (n *AlphabillNetwork) startEnterpriseTokensNode(t *testing.T) {
	container := n.startNode(t,
		"shard-node", "run",
		"--home", "/home/nonroot/5",
		"--address", "/ip4/0.0.0.0/tcp/"+containerP2pPort,
		"--rpc-server-address", "0.0.0.0:"+containerRpcPort,
		"--log-file", "stdout",
		"--log-level", "info",
		"--log-format", "text",
		"--bootnodes", n.bootstrapNode,
		"--trust-base", "/home/nonroot/root/trust-base.json",
		"--t1-timeout", "188",
	)
	n.EnterpriseTokensRpcUrl = rpcUrl(t, n.ctx, container)
}

func (n *AlphabillNetwork) startNode(t *testing.T, args ...string) tc.Container {
	waitLog := "BuildInfo="
	if args[0] == "root-node" {
		// For root node, we wait until it is ready to respond to handshakes
		waitLog = "round=4"
	}

	cr := tc.ContainerRequest{
		Image:      dockerImage(),
		WaitingFor: wait.ForLog(waitLog).WithStartupTimeout(5 * time.Second),
		LogConsumerCfg: &tc.LogConsumerConfig{
			Consumers: []tc.LogConsumer{&StdoutLogConsumer{}},
		},
		Entrypoint: []string{"/home/nonroot/alphabill.sh"},
		Files: []tc.ContainerFile{{
			HostFilePath:      "./testdata/alphabill.sh",
			ContainerFilePath: "/home/nonroot/alphabill.sh",
			FileMode:          0o755,
		}, {
			Reader:            bytes.NewReader(n.genesis),
			ContainerFilePath: containerGenesisPath,
			FileMode:          0o755,
		}},
		Cmd:          args,
		Networks:     []string{n.dockerNetwork},
		ExposedPorts: []string{containerRpcPort},
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

	return gc
}

func rpcUrl(t *testing.T, ctx context.Context, container tc.Container) string {
	rpcPort, err := container.MappedPort(ctx, containerRpcPort)
	require.NoError(t, err)
	rpcHost, err := container.Host(ctx)
	require.NoError(t, err)

	return fmt.Sprintf("http://%s:%s/rpc", rpcHost, rpcPort.Port())
}

func p2pUrl(t *testing.T, ctx context.Context, container tc.Container, homeDir string) string {
	ip, err := container.ContainerIP(ctx)
	require.NoError(t, err)

	_, r, err := container.Exec(ctx, []string{
		"alphabill", "node-id", "--home", homeDir,
	}, exec.Multiplexed())
	require.NoError(t, err)

	id, err := io.ReadAll(r)
	require.NoError(t, err)
	return fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", ip, containerP2pPort, strings.TrimSpace(string(id)))
}
