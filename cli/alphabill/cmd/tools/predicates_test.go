package tools

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func Test_createPredicateCmd(t *testing.T) {

	// create just one temp dir for the tests to share
	tmpDir := t.TempDir()

	createCmd := func(args ...string) *cobra.Command {
		cmd := NewToolsCmd()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetArgs(append([]string{"create-predicate"}, args...))
		return cmd
	}

	t.Run("no flags provided", func(t *testing.T) {
		cmd := createCmd()
		require.EqualError(t, cmd.Execute(), `reading predicate code: either "code" or "code-file" flag must be set`)
	})

	t.Run("mutually exclusive flags are used", func(t *testing.T) {
		cmd := createCmd("--code=010203", "--code-file=foo.bar")
		require.EqualError(t, cmd.Execute(), `if any flags in the group [code code-file] are set none of the others can be; [code code-file] were all set`)
	})

	t.Run("invalid inline hex", func(t *testing.T) {
		cmd := createCmd("-e=5", "--code=nope")
		require.EqualError(t, cmd.Execute(), `invalid argument "nope" for "-c, --code" flag: encoding/hex: invalid byte: U+006E 'n'`)
	})

	t.Run("input file doesnt exist", func(t *testing.T) {
		fn := filepath.Join(tmpDir, "foo.bar")
		cmd := createCmd("--code=01", "--parameter-file="+fn)
		err := cmd.Execute()
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("inline values, no params", func(t *testing.T) {
		out := bytes.NewBuffer(nil)
		cmd := createCmd("-e=0", "--code=01") // == always true template
		cmd.SetOut(out)
		require.NoError(t, cmd.Execute())
		require.Equal(t, []byte{0x83, 0x0, 0x41, 0x1, 0xf6}, out.Bytes())
	})

	t.Run("inline values", func(t *testing.T) {
		out := bytes.NewBuffer(nil)
		cmd := createCmd("-e=5", "--code=010203", "--parameter=abcd")
		cmd.SetOut(out)
		require.NoError(t, cmd.Execute())
		require.Equal(t, []byte{0x83, 5, 0x43, 1, 2, 3, 0x42, 0xAB, 0xCD}, out.Bytes())
	})

	t.Run("read params from file", func(t *testing.T) {
		fn := filepath.Join(tmpDir, "params.bin")
		require.NoError(t, os.WriteFile(fn, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}, 0666))

		out := bytes.NewBuffer(nil)
		cmd := createCmd("-e=3", "--code=00", "--parameter-file="+fn)
		cmd.SetOut(out)
		require.NoError(t, cmd.Execute())
		require.Equal(t, []byte{0x83, 3, 0x41, 0, 0x4a, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0}, out.Bytes())
	})

	t.Run("save output into file", func(t *testing.T) {
		fn := filepath.Join(tmpDir, "predicate.cbor")
		cmd := createCmd("-e=10", "--code=CC", "--output="+fn)
		require.NoError(t, cmd.Execute())
		data, err := os.ReadFile(fn)
		require.NoError(t, err)
		require.Equal(t, []byte{0x83, 10, 0x41, 0xCC, 0xf6}, data)
	})
}
