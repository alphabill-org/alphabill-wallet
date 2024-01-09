package types

import (
	"math"
	"time"

	grpckeepalive "google.golang.org/grpc/keepalive"
)

type (
	// GrpcServerConfiguration is a common configuration for gRPC servers.
	GrpcServerConfiguration struct {
		// Listen address together with port.
		Address string `validate:"empty=false"`

		// Maximum number of bytes the incoming message may be.
		MaxRecvMsgSize int `validate:"gte=0" default:"4194304"`

		// Maximum number of bytes the outgoing message may be.
		MaxSendMsgSize int `validate:"gte=0" default:"2147483647"`

		// MaxConnectionAgeMs is a duration for the maximum amount of time a
		// connection may exist before it will be closed by sending a GoAway. A
		// random jitter of +/-10% will be added to MaxConnectionAgeMs to spread out
		// connection storms.
		MaxConnectionAgeMs int64

		// MaxConnectionAgeGraceMs is an additive period after MaxConnectionAgeMs after
		// which the connection will be forcibly closed.
		MaxConnectionAgeGraceMs int64

		// MaxGetBlocksBatchSize is the max allowed block count for the GetBlocks rpc function.
		MaxGetBlocksBatchSize uint64
	}
)

const (
	DefaultMaxRecvMsgSize        = 1024 * 1024 * 4
	DefaultMaxSendMsgSize        = math.MaxInt32
	DefaultMaxGetBlocksBatchSize = 100
)

func (c *GrpcServerConfiguration) GrpcKeepAliveServerParameters() grpckeepalive.ServerParameters {
	p := grpckeepalive.ServerParameters{}
	if c.MaxConnectionAgeMs != 0 {
		p.MaxConnectionAge = time.Duration(c.MaxConnectionAgeMs) * time.Millisecond
	}
	if c.MaxConnectionAgeGraceMs != 0 {
		p.MaxConnectionAgeGrace = time.Duration(c.MaxConnectionAgeGraceMs) * time.Millisecond
	}
	return p
}
