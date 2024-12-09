package mocksrv

import (
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-wallet/client/types"
)

type (
	AdminServiceMock struct {
		InfoResponse *types.NodeInfoResponse
	}
)

func NewAdminServiceMock(opts ...Option) *AdminServiceMock {
	options := &Options{
		InfoResponse: &types.NodeInfoResponse{
			PartitionID:     1,
			PartitionTypeID: money.PartitionTypeID,
			Self:            types.PeerInfo{Identifier: "1337", Addresses: make([]string, 0)},
		},
	}
	for _, option := range opts {
		option(options)
	}
	return &AdminServiceMock{
		InfoResponse: options.InfoResponse,
	}
}

func WithInfoResponse(infoResponse *types.NodeInfoResponse) Option {
	return func(o *Options) {
		o.InfoResponse = infoResponse
	}
}

func (s *AdminServiceMock) GetNodeInfo() (*types.NodeInfoResponse, error) {
	return s.InfoResponse, nil
}
