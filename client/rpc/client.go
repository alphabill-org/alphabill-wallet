package rpc

import (
	"context"
	"fmt"
	"time"

	ethrpc "github.com/ethereum/go-ethereum/rpc"
)

const (
	defaultBatchItemLimit = 100
	defaultRetryCount     = 5
	defaultRetryTime      = time.Second
)

type (
	Client struct {
		rpcClient *ethrpc.Client
		options   *Options
	}

	Options struct {
		batchItemLimit int
		retryCount     int
		retryTime      time.Duration
	}

	Option func(*Options)
)

func WithBatchItemLimit(batchItemLimit int) Option {
	return func(o *Options) {
		o.batchItemLimit = batchItemLimit
	}
}

func WithRetryCount(retryCount int) Option {
	return func(o *Options) {
		o.retryCount = retryCount
	}
}

func WithRetryTime(retryTime time.Duration) Option {
	return func(o *Options) {
		o.retryTime = retryTime
	}
}

func NewClient(ctx context.Context, rpcUrl string, opts ...Option) (*Client, error) {
	options := optionsWithDefaults(opts)

	rpcClient, err := ethrpc.DialContext(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}

	return &Client{
		rpcClient: rpcClient,
		options:   options,
	}, nil
}

func (c *Client) BatchCall(ctx context.Context, batch []*ethrpc.BatchElem) error {
	start, end := 0, 0
	for len(batch) > end {
		if c.options.batchItemLimit == 0 {
			end = len(batch)
		} else {
			end = min(len(batch), start+c.options.batchItemLimit)
		}
		if err := c.batchCallWithRetry(ctx, batch[start:end]); err != nil {
			return fmt.Errorf("failed to send batch request: %w", err)
		}
		start = end
	}
	return nil
}

func (c *Client) batchCallWithRetry(ctx context.Context, batch []*ethrpc.BatchElem) error {
	for countdown := c.options.retryCount; ; countdown-- {
		err := c.batchCallContext(ctx, batch)
		if err == nil {
			var retryBatch []*ethrpc.BatchElem
			for _, elem := range batch {
				if elem.Error != nil {
					retryBatch = append(retryBatch, elem)
				}
			}
			if len(retryBatch) == 0 || countdown <= 0 {
				return nil
			}
			batch = retryBatch
		}

		if countdown <= 0 {
			return fmt.Errorf("batch request failed (retries %d): err %w", c.options.retryCount, err)
		}

		select {
		case <-time.After(c.options.retryTime):
			continue
		case <-ctx.Done():
			return fmt.Errorf("batch request failed: err %w", ctx.Err())
		}
	}
}

// batchCallContext is wrapper form batch []*ethrpc.BatchElem to batch []ethrpc.BatchElem
func (c *Client) batchCallContext(ctx context.Context, batch []*ethrpc.BatchElem) error {
	batchToSend := make([]ethrpc.BatchElem, len(batch))
	for i, element := range batch {
		batchToSend[i] = *element
	}
	err := c.rpcClient.BatchCallContext(ctx, batchToSend)
	for i, element := range batchToSend {
		batch[i].Result = element.Result
		batch[i].Error = element.Error
	}
	return err
}

func (c *Client) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	for countdown := c.options.retryCount; ; countdown-- {
		err := c.rpcClient.CallContext(ctx, result, method, args...)
		if err == nil || countdown <= 0 {
			return err
		}

		select {
		case <-time.After(c.options.retryTime):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *Client) Close() {
	c.rpcClient.Close()
}

func optionsWithDefaults(opts []Option) *Options {
	res := &Options{
		batchItemLimit: defaultBatchItemLimit,
		retryCount:     defaultRetryCount,
		retryTime:      defaultRetryTime,
	}
	for _, opt := range opts {
		opt(res)
	}
	return res
}
