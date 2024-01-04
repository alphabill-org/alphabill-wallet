package testutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

const walletBaseDir = "wallet"

var fcrID = money.NewFeeCreditRecordID(nil, []byte{1})

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

func SetupTestHomeDir(t *testing.T, dir string) string {
	outputDir := filepath.Join(t.TempDir(), dir)
	err := os.MkdirAll(outputDir, 0700) // -rwx------
	require.NoError(t, err)
	return outputDir
}

func VerifyStdout(t *testing.T, consoleWriter *TestConsoleWriter, expectedLines ...string) {
	joined := consoleWriter.String()
	for _, expectedLine := range expectedLines {
		require.Contains(t, joined, expectedLine)
	}
}
