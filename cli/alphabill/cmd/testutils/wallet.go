package testutils

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/stretchr/testify/require"

	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	tokenswallet "github.com/alphabill-org/alphabill-wallet/wallet/tokens"
	tokensclient "github.com/alphabill-org/alphabill-wallet/wallet/tokens/client"
)

const walletBaseDir = "wallet"

func CreateNewTestWallet(t *testing.T) string {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, walletBaseDir)
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	defer am.Close()
	err = am.CreateKeys("")
	require.NoError(t, err)
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
	clientURL, err := url.Parse(addr)
	require.NoError(t, err)
	backendClient := tokensclient.New(*clientURL, o.DefaultObserver())
	w, err := tokenswallet.New(tokens.DefaultSystemIdentifier, backendClient, am, false, nil, o.DefaultLogger())
	require.NoError(t, err)
	require.NotNil(t, w)

	return w, homeDir
}

func SetupTestHomeDir(t *testing.T, dir string) string {
	outputDir := filepath.Join(t.TempDir(), dir)
	err := os.MkdirAll(outputDir, 0700) // -rwx------
	require.NoError(t, err)
	return outputDir
}
