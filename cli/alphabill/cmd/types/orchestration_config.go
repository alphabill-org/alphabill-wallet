package types

import "github.com/alphabill-org/alphabill-go-base/types"

type (
	OrchestrationConfig struct {
		WalletConfig *WalletConfig
		RpcUrl       string
		Key          uint64
	}

	AddVarCmdConfig struct {
		OrchestrationConfig *OrchestrationConfig
		PartitionID         uint32
		ShardID             types.ShardID
		VarFilePath         string
	}
)
