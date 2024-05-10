package tools

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates"
	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	"github.com/alphabill-org/alphabill-go-base/types"
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

	t.Run("input file doesn't exist", func(t *testing.T) {
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

func Test_createWASMPredicateCmd(t *testing.T) {

	// create just one temp dir for the tests to share
	tmpDir := t.TempDir()

	createCmd := func(args ...string) *cobra.Command {
		cmd := NewToolsCmd()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetArgs(append([]string{"create-wasm-predicate"}, args...))
		return cmd
	}

	t.Run("no flags provided", func(t *testing.T) {
		cmd := createCmd()
		require.EqualError(t, cmd.Execute(), `required flag(s) "code-file", "entrypoint" not set`)
	})

	t.Run("minimum required flags", func(t *testing.T) {
		code := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
		codeFile := filepath.Join(tmpDir, "code.bin")
		require.NoError(t, os.WriteFile(codeFile, code, 0666))

		out := bytes.NewBuffer(nil)
		cmd := createCmd("--code-file="+codeFile, "--entrypoint=func-name")
		cmd.SetOut(out)
		require.NoError(t, cmd.Execute())

		pred := predicates.Predicate{}
		require.NoError(t, types.Cbor.Decode(out, &pred))
		require.EqualValues(t, wasm.PredicateEngineID, pred.Tag)
		require.Equal(t, code, pred.Code)
		wp := wasm.PredicateParams{}
		require.NoError(t, types.Cbor.Unmarshal(pred.Params, &wp))
		require.Equal(t, "func-name", wp.Entrypoint)
		require.Empty(t, wp.Args)
	})

	t.Run("all flags", func(t *testing.T) {
		code := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
		codeFile := filepath.Join(tmpDir, "code.bin")
		require.NoError(t, os.WriteFile(codeFile, code, 0666))

		argsFile := filepath.Join(tmpDir, "args.plist")
		require.NoError(t, os.WriteFile(argsFile, []byte(`(<*I50>, <20ab30bc>)`), 0666))

		out := bytes.NewBuffer(nil)
		cmd := createCmd("--code-file="+codeFile, "--entrypoint=func-name", "--parameter-file="+argsFile)
		cmd.SetOut(out)
		require.NoError(t, cmd.Execute())

		pred := predicates.Predicate{}
		require.NoError(t, types.Cbor.Decode(out, &pred))
		require.EqualValues(t, wasm.PredicateEngineID, pred.Tag)
		require.Equal(t, code, pred.Code)
		wp := wasm.PredicateParams{}
		require.NoError(t, types.Cbor.Unmarshal(pred.Params, &wp))
		require.Equal(t, "func-name", wp.Entrypoint)
		// CBOR(8218324420ab30bc) == [50, h'20AB30BC']
		require.Equal(t, []byte{0x82, 0x18, 0x32, 0x44, 0x20, 0xab, 0x30, 0xbc}, wp.Args, "ARGS: %x", wp.Args)
	})
}

func Test_convertToCBOR(t *testing.T) {
	// decode "data" from hex encoded string to binary
	asBytes := func(t *testing.T, data string) []byte {
		bin, err := hex.DecodeString(data)
		require.NoError(t, err)
		return bin
	}

	t.Run("unknown type", func(t *testing.T) {
		// we expect to get the exact input back (doesn't need to be valid CBOR)
		in := []byte{10, 20, 30, 40, 50}
		data, err := convertToCBOR(in, "")
		require.NoError(t, err)
		require.Equal(t, in, data, "hex %x", data)

		// nil in, nil out
		in = nil
		data, err = convertToCBOR(in, ".nil")
		require.NoError(t, err)
		require.EqualValues(t, in, data)
	})

	t.Run("plist", func(t *testing.T) {
		// input is plist GNUstep format
		// [10, "str"]
		data, err := convertToCBOR([]byte(`(<*I10>, "str")`), filepath.Ext("foo/bar.plist"))
		require.NoError(t, err)
		require.Equal(t, asBytes(t, "820a63737472"), data, "CBOR as hex: %x", data)

		// [10, "str", h'0102FF']
		data, err = convertToCBOR([]byte(`(<*I10>, "str", <01 02 ff>)`), filepath.Ext("bar.plist"))
		require.NoError(t, err)
		require.Equal(t, asBytes(t, "830a63737472430102ff"), data, "CBOR as hex: %x", data)

		plist := `(
			/* integer 200 */
			<*I200>,
			/* date type support */
			<*D2024-05-20 12:30:00 +0100>,
			/* binary data as hex */
			<01020305060708090a>
		)`
		data, err = convertToCBOR([]byte(plist), filepath.Ext("../bar.plist"))
		require.NoError(t, err)
		// [200, 1716204600, h'01020305060708090A']
		// 1716204600 is Epoch Unix Timestamp (seconds) = Mon May 20 2024 11:30:00 GMT+0000
		require.Equal(t, asBytes(t, "8318c81a664b34384901020305060708090a"), data, "CBOR as hex: %x", data)
	})

	t.Run("YAML", func(t *testing.T) {
		// [10, "str"]
		data, err := convertToCBOR([]byte("- 10\n- str"), filepath.Ext("foo.bar.yaml"))
		require.NoError(t, err)
		require.Equal(t, asBytes(t, "820a63737472"), data)

		// yaml kinda supports binary data (encoded as base64) but it ends up as string, not bytes :(
		bin := []byte{42, 37, 99}
		b64 := base64.StdEncoding.EncodeToString(bin)
		data, err = convertToCBOR([]byte("raw: !!binary "+b64), filepath.Ext("./bar.yaml"))
		require.NoError(t, err)
		// results in
		// {"raw": "*%c"}
		// which is a map with single item, key "raw" value "*%c" (both text(3) type)
		require.Equal(t, asBytes(t, "a163726177632a2563"), data, "CBOR as hex: %x", data)
	})

	t.Run("JSON", func(t *testing.T) {
		data, err := convertToCBOR([]byte(`[10, "str"]`), filepath.Ext("test.json"))
		require.NoError(t, err)
		// we'll get [10.0, "str"] ie in the input first element of the array is
		// integer but Go json decoder decodes all numbers into float!
		require.Equal(t, asBytes(t, "82f9490063737472"), data)
	})
}
