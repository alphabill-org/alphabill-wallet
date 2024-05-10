package tools

import (
	"github.com/spf13/cobra"
)

func NewToolsCmd() *cobra.Command {
	toolsCmd := &cobra.Command{
		Use:   "tool",
		Short: "tools working with Alphabill data structures etc",
	}
	toolsCmd.AddCommand(createPredicateCmd())
	toolsCmd.AddCommand(createWASMPredicateCmd())

	return toolsCmd
}
