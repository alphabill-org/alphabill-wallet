package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"howett.net/plist"

	"github.com/alphabill-org/alphabill-go-base/predicates"
	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	"github.com/alphabill-org/alphabill-go-base/types"
)

const (
	flagNameEngine    = "engine"
	flagNameCode      = "code"
	flagNameCodeFile  = "code-file"
	flagNameMainFunc  = "entrypoint"
	flagNameParam     = "parameter"
	flagNameParamFile = "parameter-file"
	flagNameOutput    = "output"
	flagHelpOutput    = "filename into which to save the predicate record, if not set then stdout"
)

func createPredicateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-predicate",
		Short: "Create predicate record out of parts",
		Long: `This is "generic" tool to compose predicate record, predicate engine specific tools ` +
			`(ie "create-wasm-predicate") might be easier to use. Using this tool requires intimate ` +
			`knowledge about the predicate engine in order to correctly compose values for the ` +
			`"code" and "parameter" flags.`,
		Example: "\ttool create-predicate --engine=0 --code=01\nwould create predicate " +
			`equivalent to the "always true" predicate template.`,
		RunE: runCreatePredicateCmd,
	}
	cmd.Flags().Uint64P(flagNameEngine, "e", 0, "predicate engine ID")

	cmd.Flags().BytesHexP(flagNameCode, "c", nil, "predicate code as hex encoded string")
	cmd.Flags().String(flagNameCodeFile, "", "filename from where to read predicate code")
	cmd.MarkFlagsMutuallyExclusive(flagNameCode, flagNameCodeFile)

	cmd.Flags().BytesHexP(flagNameParam, "p", nil, "predicate parameter as hex encoded string")
	cmd.Flags().String(flagNameParamFile, "", "filename from where to read predicate parameter")
	cmd.MarkFlagsMutuallyExclusive(flagNameParam, flagNameParamFile)

	cmd.Flags().String(flagNameOutput, "", flagHelpOutput)
	return cmd
}

func runCreatePredicateCmd(cmd *cobra.Command, args []string) (err error) {
	pred := predicates.Predicate{}
	if pred.Tag, err = cmd.Flags().GetUint64(flagNameEngine); err != nil {
		return fmt.Errorf("reading engine id flag: %w", err)
	}
	if pred.Code, err = argValueBytes(cmd, flagNameCode, flagNameCodeFile, true); err != nil {
		return fmt.Errorf("reading predicate code: %w", err)
	}
	if pred.Params, err = argValueBytes(cmd, flagNameParam, flagNameParamFile, false); err != nil {
		return fmt.Errorf("reading predicate parameter: %w", err)
	}

	if err := outputAsCBOR(cmd, pred); err != nil {
		return fmt.Errorf("encoding predicate as CBOR: %w", err)
	}
	return nil
}

func createWASMPredicateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "create-wasm-predicate",
		Short: `Create predicate record out of parts, similar to the 'create-predicate' tool but ` +
			`specific for creating WASM predicates.`,
		Long: `This tool creates "predicate record" for custom predicate implemented as WASM module. ` +
			`The output of the tool can be used as input for wallet commands which accept predicates ` +
			`ie "mint-clause" flag on "wallet token new-type fungible" command.`,
		Example: fmt.Sprintf("abwallet tool create-wasm-predicate --%s=./prg/predicate.wasm --%s=bearer_invariant --%s=./prg/bearer_invariant.cbor", flagNameCodeFile, flagNameMainFunc, flagNameOutput),
		RunE:    runCreateWASMPredicateCmd,
	}

	cmd.Flags().String(flagNameCodeFile, "", "filename from where to read predicate code (WASM binary)")
	if err := cmd.MarkFlagRequired(flagNameCodeFile); err != nil {
		panic(err)
	}
	cmd.Flags().String(flagNameMainFunc, "", `name of the function to call from the WASM binary, ie `+
		`the function which implements the predicate logic`)
	if err := cmd.MarkFlagRequired(flagNameMainFunc); err != nil {
		panic(err)
	}

	cmd.Flags().BytesHexP(flagNameParam, "p", nil, "predicate program parameter as hex encoded string, see '"+
		flagNameParamFile+"' flag description for more.")
	cmd.Flags().String(flagNameParamFile, "", "filename from where to read predicate program parameter - the "+
		"content of the file is stored with the program and can't be changed later, the intended use case is "+
		"to provide configuration for the predicate program. Files with extension 'plist', 'json' and 'yaml' "+
		"are first parsed as given format and then encoded as CBOR which is then stored as part of the predicate.")
	cmd.MarkFlagsMutuallyExclusive(flagNameParam, flagNameParamFile)

	cmd.Flags().String(flagNameOutput, "", flagHelpOutput)
	return cmd
}

func runCreateWASMPredicateCmd(cmd *cobra.Command, args []string) (err error) {
	wasmArg := wasm.PredicateParams{}
	if wasmArg.Entrypoint, err = cmd.Flags().GetString(flagNameMainFunc); err != nil {
		return fmt.Errorf("reading %q parameter: %w", flagNameMainFunc, err)
	}
	if wasmArg.Args, err = argWasmPredicateParam(cmd, flagNameParam, flagNameParamFile); err != nil {
		return fmt.Errorf("reading wasm arguments: %w", err)
	}
	if err := wasmArg.IsValid(); err != nil {
		return fmt.Errorf("invalid WASM predicate parameters: %w", err)
	}

	pred := predicates.Predicate{Tag: wasm.PredicateEngineID}
	if pred.Code, err = argValueBytes(cmd, flagNameCode, flagNameCodeFile, true); err != nil {
		return fmt.Errorf("reading predicate code: %w", err)
	}
	if pred.Params, err = types.Cbor.Marshal(wasmArg); err != nil {
		return fmt.Errorf("encoding predicate parameters as CBOR: %w", err)
	}

	if err := outputAsCBOR(cmd, pred); err != nil {
		return fmt.Errorf("encoding predicate as CBOR: %w", err)
	}
	return nil
}

func argWasmPredicateParam(cmd *cobra.Command, flagHexStr, flagFilename string) (data []byte, err error) {
	if data, err = argValueBytes(cmd, flagHexStr, flagFilename, false); err != nil {
		return nil, fmt.Errorf("reading data from file: %w", err)
	}

	if data != nil && cmd.Flags().Changed(flagFilename) {
		filename, err := cmd.Flags().GetString(flagFilename)
		if err != nil {
			return nil, fmt.Errorf("reading %q flag to determine input data type: %w", flagFilename, err)
		}
		return convertToCBOR(data, filepath.Ext(filename))
	}

	return data, nil
}

/*
convertToCBOR re-encodes input "data" as CBOR if the "format" is one of the
supported formats, otherwise the data will be returned as-is. Supported
formats are "plist", "json" and "yaml", the name is not case sensitive and
may have dot as a prefix (ie it's OK to pass `filepath.Ext(filename)` for
format).
Passing "unsupported" format name is not an error, the original data is
returned. Only failure to decode or encode will cause non-nil error return.
*/
func convertToCBOR(data []byte, format string) (_ []byte, err error) {
	var v any
	switch strings.ToLower(strings.TrimLeft(format, ".")) {
	case "plist":
		if _, err := plist.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("decoding data as plist: %w", err)
		}
	case "json":
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("decoding data as json: %w", err)
		}
	case "yaml":
		if err := yaml.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("decoding data as yaml: %w", err)
		}
	default:
		return data, nil
	}

	if data, err = types.Cbor.Marshal(v); err != nil {
		return nil, fmt.Errorf("encoding data as CBOR: %w", err)
	}
	return data, nil
}

/*
outputAsCBOR encodes "data" as CBOR and saves it to "output".
The "output" is either the filename set using "flagNameOutput" flag
or the output set for the "cmd" (stdout by default).
*/
func outputAsCBOR(cmd *cobra.Command, data any) error {
	var out io.Writer = cmd.OutOrStdout()
	if cmd.Flags().Changed(flagNameOutput) {
		filename, err := cmd.Flags().GetString(flagNameOutput)
		if err != nil {
			return fmt.Errorf("reading flag %q value: %w", flagNameOutput, err)
		}
		f, err := os.Create(filepath.Clean(filename))
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		out = f
	}

	if err := types.Cbor.Encode(out, data); err != nil {
		return fmt.Errorf("encoding as CBOR: %w", err)
	}
	return nil
}

/*
argValueBytes is meant to be used in situation where there is two mutually exclusive flags to
provide the same (BLOB) argument - one for reading hex encoded value from cmd line and other
to read content of a file.
When "required" is "true" error is returned if neither of the flag is set.
*/
func argValueBytes(cmd *cobra.Command, hexFlag, fileFlag string, required bool) ([]byte, error) {
	if cmd.Flags().Changed(hexFlag) {
		code, err := cmd.Flags().GetBytesHex(hexFlag)
		if err != nil {
			return nil, fmt.Errorf("reading %q flag: %w", hexFlag, err)
		}
		return code, nil
	}

	if cmd.Flags().Changed(fileFlag) {
		s, err := cmd.Flags().GetString(fileFlag)
		if err != nil {
			return nil, fmt.Errorf("reading %q flag: %w", fileFlag, err)
		}
		filename, err := filepath.Abs(s)
		if err != nil {
			return nil, fmt.Errorf("parsing %q flag as filename: %w", fileFlag, err)
		}
		buf, err := os.ReadFile(filepath.Clean(filename))
		if err != nil {
			return nil, fmt.Errorf("reading %q file: %w", filename, err)
		}
		return buf, nil
	}

	if required {
		return nil, fmt.Errorf("either %q or %q flag must be set", hexFlag, fileFlag)
	}
	return nil, nil
}
