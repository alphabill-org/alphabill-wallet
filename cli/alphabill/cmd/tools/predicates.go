package tools

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/types"
)

const (
	flagNameEngine    = "engine"
	flagNameCode      = "code"
	flagNameCodeFile  = "code-file"
	flagNameParam     = "parameter"
	flagNameParamFile = "parameter-file"
	flagNameOutput    = "output"
)

func createPredicateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create-predicate",
		Short:   "Create predicate record out of parts",
		Example: `command "tool create-predicate -e=0 --code=01" would create "always true" predicate template.`,
		RunE:    runCreatePredicateCmd,
	}
	cmd.Flags().Uint64P(flagNameEngine, "e", 0, "predicate engine ID")

	cmd.Flags().BytesHexP(flagNameCode, "c", nil, "predicate code as hex encoded string")
	cmd.Flags().String(flagNameCodeFile, "", "filename from where to read predicate code")
	cmd.MarkFlagsMutuallyExclusive(flagNameCode, flagNameCodeFile)

	cmd.Flags().BytesHexP(flagNameParam, "p", nil, "predicate parameter as hex encoded string")
	cmd.Flags().String(flagNameParamFile, "", "filename from where to read predicate parameter")
	cmd.MarkFlagsMutuallyExclusive(flagNameParam, flagNameParamFile)

	cmd.Flags().String(flagNameOutput, "", "filename into which to save the predicate record, if not set then stdout")
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

	if err := types.Cbor.Encode(out, pred); err != nil {
		return fmt.Errorf("encoding predicate as CBOR: %w", err)
	}
	return nil
}

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
