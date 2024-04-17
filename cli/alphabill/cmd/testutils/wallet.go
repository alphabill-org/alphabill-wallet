package testutils

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/hash"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	tokenswallet "github.com/alphabill-org/alphabill-wallet/wallet/tokens"
)

const (
	walletBaseDir  = "wallet"
	TestMnemonic   = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"
	TestPubKey0Hex = "03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	TestPubKey1Hex = "02d36c574db299904b285aaeb57eb7b1fa145c43af90bec3c635c4174c224587b6"
)

func CreateNewTestWallet(t *testing.T, opts ...Option) string {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, walletBaseDir)
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	defer am.Close()
	err = am.CreateKeys(options.mnemonic)
	require.NoError(t, err)

	for i := 1; i < options.numberOfAccounts; i++ {
		_, _, err := am.AddAccount()
		require.NoError(t, err)
	}
	return homeDir
}

func CreateNewWallet(t *testing.T) (account.Manager, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, walletBaseDir)
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	t.Cleanup(am.Close)
	err = am.CreateKeys("")
	require.NoError(t, err)
	return am, homeDir
}

func CreateNewTokenWallet(t *testing.T, addr string) (*tokenswallet.Wallet, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	require.NoError(t, am.CreateKeys(""))

	o := testobserve.NewFactory(t)
	rpcClient, err := rpc.DialContext(context.Background(), args.BuildRpcUrl(addr))
	require.NoError(t, err)
	t.Cleanup(rpcClient.Close)
	tokensRpcClient := rpc.NewTokensClient(rpcClient)
	w, err := tokenswallet.New(tokens.DefaultSystemID, tokensRpcClient, am, false, nil, o.DefaultLogger())
	require.NoError(t, err)
	require.NotNil(t, w)
	t.Cleanup(w.Shutdown)

	return w, homeDir
}

func SetupTestHomeDir(t *testing.T, dir string) string {
	outputDir := filepath.Join(t.TempDir(), dir)
	err := os.MkdirAll(outputDir, 0700) // -rwx------
	require.NoError(t, err)
	return outputDir
}

func TestPubKey0Hash(t *testing.T) []byte {
	pubKeyBytes, err := hex.DecodeString(TestPubKey0Hex)
	require.NoError(t, err)
	return hash.Sum256(pubKeyBytes)
}

func TestPubKey1Hash(t *testing.T) []byte {
	pubKeyBytes, err := hex.DecodeString(TestPubKey1Hex)
	require.NoError(t, err)
	return hash.Sum256(pubKeyBytes)
}

type (
	Options struct {
		numberOfAccounts int
		mnemonic         string
	}

	Option func(*Options)
)

func WithDefaultMnemonic() Option {
	return func(o *Options) {
		o.mnemonic = TestMnemonic
	}
}

func WithNumberOfAccounts(numberOfAccounts int) Option {
	return func(o *Options) {
		o.numberOfAccounts = numberOfAccounts
	}
}
