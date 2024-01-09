package testutils

import (
	"math"
	"time"

	grpckeepalive "google.golang.org/grpc/keepalive"
)

type (
	// grpcServerConfiguration is a common configuration for gRPC servers.
	grpcServerConfiguration struct {
		// Listen address together with port.
		address string `validate:"empty=false"`

		// Maximum number of bytes the incoming message may be.
		maxRecvMsgSize int `validate:"gte=0" default:"4194304"`

		// Maximum number of bytes the outgoing message may be.
		maxSendMsgSize int `validate:"gte=0" default:"2147483647"`

		// maxConnectionAgeMs is a duration for the maximum amount of time a
		// connection may exist before it will be closed by sending a GoAway. A
		// random jitter of +/-10% will be added to maxConnectionAgeMs to spread out
		// connection storms.
		maxConnectionAgeMs int64

		// maxConnectionAgeGraceMs is an additive period after maxConnectionAgeMs after
		// which the connection will be forcibly closed.
		maxConnectionAgeGraceMs int64

		// maxGetBlocksBatchSize is the max allowed block count for the GetBlocks rpc function.
		maxGetBlocksBatchSize uint64
	}
)

const (
	defaultMaxRecvMsgSize        = 1024 * 1024 * 4
	defaultMaxSendMsgSize        = math.MaxInt32
	defaultMaxGetBlocksBatchSize = 100
)

func (c *grpcServerConfiguration) grpcKeepAliveServerParameters() grpckeepalive.ServerParameters {
	p := grpckeepalive.ServerParameters{}
	if c.maxConnectionAgeMs != 0 {
		p.MaxConnectionAge = time.Duration(c.maxConnectionAgeMs) * time.Millisecond
	}
	if c.maxConnectionAgeGraceMs != 0 {
		p.MaxConnectionAgeGrace = time.Duration(c.maxConnectionAgeGraceMs) * time.Millisecond
	}
	return p
}
