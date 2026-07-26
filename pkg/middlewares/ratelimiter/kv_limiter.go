package ratelimiter

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hanzokv/go/v9"
	"github.com/rs/zerolog"
	ptypes "github.com/hanzoai/ingress-parser/types"
	"github.com/hanzoai/ingress/pkg/config/dynamic"
	"golang.org/x/time/rate"
)

const kvPrefix = "rate:"

type kvLimiter struct {
	rate     rate.Limit // reqs/s
	burst    int64
	maxDelay time.Duration
	period   ptypes.Duration
	logger   *zerolog.Logger
	ttl      int
	client   KVClient
}

func newKVLimiter(ctx context.Context, rate rate.Limit, burst int64, maxDelay time.Duration, ttl int, config dynamic.RateLimit, logger *zerolog.Logger) (limiter, error) {
	options := &kv.UniversalOptions{
		Addrs:          config.Redis.Endpoints,
		Username:       config.Redis.Username,
		Password:       config.Redis.Password,
		DB:             config.Redis.DB,
		PoolSize:       config.Redis.PoolSize,
		MinIdleConns:   config.Redis.MinIdleConns,
		MaxActiveConns: config.Redis.MaxActiveConns,
	}

	if config.Redis.DialTimeout != nil && *config.Redis.DialTimeout > 0 {
		options.DialTimeout = time.Duration(*config.Redis.DialTimeout)
	}

	if config.Redis.ReadTimeout != nil {
		if *config.Redis.ReadTimeout > 0 {
			options.ReadTimeout = time.Duration(*config.Redis.ReadTimeout)
		} else {
			options.ReadTimeout = -1
		}
	}

	if config.Redis.WriteTimeout != nil {
		if *config.Redis.ReadTimeout > 0 {
			options.WriteTimeout = time.Duration(*config.Redis.WriteTimeout)
		} else {
			options.WriteTimeout = -1
		}
	}

	if config.Redis.TLS != nil {
		var err error
		options.TLSConfig, err = config.Redis.TLS.CreateTLSConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("creating TLS config: %w", err)
		}
	}

	return &kvLimiter{
		rate:     rate,
		burst:    burst,
		period:   config.Period,
		maxDelay: maxDelay,
		logger:   logger,
		ttl:      ttl,
		client:   kv.NewUniversalClient(options),
	}, nil
}

func (r *kvLimiter) Allow(ctx context.Context, source string) (*time.Duration, error) {
	ok, delay, err := r.evaluateScript(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("evaluating script: %w", err)
	}
	if !ok {
		return nil, nil
	}
	return delay, nil
}

func (r *kvLimiter) evaluateScript(ctx context.Context, key string) (bool, *time.Duration, error) {
	if r.rate == rate.Inf {
		return true, nil, nil
	}

	params := []any{
		float64(r.rate / 1000000),
		r.burst,
		r.ttl,
		time.Now().UnixMicro(),
		r.maxDelay.Microseconds(),
	}
	v, err := AllowTokenBucketScript.Run(ctx, r.client, []string{kvPrefix + key}, params...).Result()
	if err != nil {
		return false, nil, fmt.Errorf("running script: %w", err)
	}

	values := v.([]any)
	ok, err := strconv.ParseBool(values[0].(string))
	if err != nil {
		return false, nil, fmt.Errorf("parsing ok value from kv rate lua script: %w", err)
	}
	delay, err := strconv.ParseFloat(values[1].(string), 64)
	if err != nil {
		return false, nil, fmt.Errorf("parsing delay value from kv rate lua script: %w", err)
	}

	microDelay := time.Duration(delay * float64(time.Microsecond))
	return ok, &microDelay, nil
}
