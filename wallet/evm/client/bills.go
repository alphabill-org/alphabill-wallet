package client

type (
	Bill struct {
		Id      []byte `json:"id"`
		Value   uint64 `json:"value,string"`
		TxHash  []byte `json:"txHash"`
		Counter uint64 `json:"counter,string"`
	}

	RoundNumber struct {
		RoundNumber            uint64 `json:"roundNumber,string"`            // last known round number
		LastIndexedRoundNumber uint64 `json:"lastIndexedRoundNumber,string"` // last indexed round number
	}
)

func (x *Bill) GetID() []byte {
	if x != nil {
		return x.Id
	}
	return nil
}

func (x *Bill) GetValue() uint64 {
	if x != nil {
		return x.Value
	}
	return 0
}

func (x *Bill) GetTxHash() []byte {
	if x != nil {
		return x.TxHash
	}
	return nil
}
