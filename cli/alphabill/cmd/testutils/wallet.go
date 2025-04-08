package testutils

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

const (
	WalletBaseDir  = "wallet"
	TestMnemonic   = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"
	TestPubKey0Hex = "03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	TestPubKey1Hex = "02d36c574db299904b285aaeb57eb7b1fa145c43af90bec3c635c4174c224587b6"
	WaitDuration   = 4 * time.Second
	WaitTick       = 100 * time.Millisecond
)

func CreateNewTestWallet(t *testing.T, opts ...Option) string {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, WalletBaseDir)
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

func CreateNewWallet(t *testing.T, mnemonic string) (account.Manager, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, WalletBaseDir)
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	t.Cleanup(am.Close)
	err = am.CreateKeys(mnemonic)
	require.NoError(t, err)
	return am, homeDir
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
	pkh := sha256.Sum256(pubKeyBytes)
	return pkh[:]
}

func TestPubKey1Hash(t *testing.T) []byte {
	pubKeyBytes, err := hex.DecodeString(TestPubKey1Hex)
	require.NoError(t, err)
	pkh := sha256.Sum256(pubKeyBytes)
	return pkh[:]
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
