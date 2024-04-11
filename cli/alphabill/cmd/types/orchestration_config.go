package types

type (
	OrchestrationConfig struct {
		WalletConfig *WalletConfig
		RpcUrl       string
		Key          uint64
		SystemID     uint32
	}

	AddVarCmdConfig struct {
		OrchestrationConfig *OrchestrationConfig
		PartitionID         uint32
		ShardID             BytesHex
		VarFilePath         string
	}
)
