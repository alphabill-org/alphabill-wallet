package mocksrv

import (
	abrpc "github.com/alphabill-org/alphabill/rpc"
	"github.com/multiformats/go-multiaddr"
)

type (
	AdminServiceMock struct {
		InfoResponse *abrpc.NodeInfoResponse
	}
)

func NewAdminServiceMock(opts ...Option) *AdminServiceMock {
	options := &Options{
		InfoResponse: &abrpc.NodeInfoResponse{
			SystemID: 1,
			Name:     "money node",
			Self:     abrpc.PeerInfo{Identifier: "1337", Addresses: make([]multiaddr.Multiaddr, 0)},
		},
	}
	for _, option := range opts {
		option(options)
	}
	return &AdminServiceMock{
		InfoResponse: options.InfoResponse,
	}
}

func WithInfoResponse(infoResponse *abrpc.NodeInfoResponse) Option {
	return func(o *Options) {
		o.InfoResponse = infoResponse
	}
}

func (s *AdminServiceMock) GetNodeInfo() (*abrpc.NodeInfoResponse, error) {
	return s.InfoResponse, nil
}
