package testutils

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
)

type (
	CmdConstructor    func(*types.BaseConfiguration) *cobra.Command
	SubCmdConstructor func(*types.WalletConfig) *cobra.Command
)

type CmdExecutor struct {
	home           string
	cmdConstructor CmdConstructor
	prefixArgs     []string
}

func NewCmdExecutor(cmdConstructor CmdConstructor, prefixArgs ...string) *CmdExecutor {
	return &CmdExecutor{
		cmdConstructor: cmdConstructor,
		prefixArgs:     prefixArgs,
	}
}

func NewSubCmdExecutor(cmdConstructor SubCmdConstructor, prefixArgs ...string) *CmdExecutor {
	return NewCmdExecutor(func(baseConf *types.BaseConfiguration) *cobra.Command {
		return cmdConstructor(&types.WalletConfig{
			Base: baseConf,
			WalletHomeDir: filepath.Join(baseConf.HomeDir, "wallet"),
		})
	}, prefixArgs...)
}

func (c CmdExecutor) WithHome(home string) *CmdExecutor {
	return &CmdExecutor{
		home:           home,
		cmdConstructor: c.cmdConstructor,
		prefixArgs:     c.prefixArgs,
	}
}

func (c CmdExecutor) WithPrefixArgs(prefixArgs ...string) *CmdExecutor {
	return &CmdExecutor{
		home:           c.home,
		cmdConstructor: c.cmdConstructor,
		prefixArgs:     append(c.prefixArgs, prefixArgs...),
	}
}

func (c *CmdExecutor) Exec(t *testing.T, args ...string) *TestConsoleWriter {
	output, err := c.exec(t, args...)
	require.NoError(t, err)
	return output
}

func (c *CmdExecutor) ExecFunc(t *testing.T, args ...string) func() *TestConsoleWriter {
	return func() *TestConsoleWriter {
		return c.Exec(t, args...)
	}
}

func (c *CmdExecutor) ExecWithError(t *testing.T, expectedError string, args ...string) {
	_, err := c.exec(t, args...)
	require.ErrorContains(t, err, expectedError)
}

func (c *CmdExecutor) exec(t *testing.T, args ...string) (*TestConsoleWriter, error) {
	consoleWriter := &TestConsoleWriter{}
	cmdConf := &types.BaseConfiguration{
		HomeDir:       c.home,
		ConsoleWriter: consoleWriter,
		Logger:        logger.New(t),
	}
	cmd := c.cmdConstructor(cmdConf)
	cmd.SetArgs(append(c.prefixArgs, args...))

	return consoleWriter, cmd.Execute()
}
