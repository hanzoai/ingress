package ratelimiter

import (
	"context"

	"github.com/hanzokv/go/v9"
)

type KVClient interface {
	Eval(ctx context.Context, script string, keys []string, args ...any) *kv.Cmd
	EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) *kv.Cmd
	ScriptExists(ctx context.Context, hashes ...string) *kv.BoolSliceCmd
	ScriptLoad(ctx context.Context, script string) *kv.StringCmd
	Del(ctx context.Context, keys ...string) *kv.IntCmd

	EvalRO(ctx context.Context, script string, keys []string, args ...any) *kv.Cmd
	EvalShaRO(ctx context.Context, sha1 string, keys []string, args ...any) *kv.Cmd
}

//nolint:dupword
var AllowTokenBucketRaw = `
local key = KEYS[1]
local limit, burst, ttl, t, max_delay = tonumber(ARGV[1]), tonumber(ARGV[2]), tonumber(ARGV[3]), tonumber(ARGV[4]),
    tonumber(ARGV[5])

local bucket = {
    limit = limit,
    burst = burst,
    tokens = 0,
    last = 0
}

local rl_source = redis.call('hgetall', key)

if table.maxn(rl_source) == 4 then
    -- Get bucket state from redis
    bucket.last = tonumber(rl_source[2])
    bucket.tokens = tonumber(rl_source[4])
end

local last = bucket.last
if t < last then
    last = t
end

local elapsed = t - last
local delta = bucket.limit * elapsed
local tokens = bucket.tokens + delta
tokens = math.min(tokens, bucket.burst)
tokens = tokens - 1

local wait_duration = 0
if tokens < 0 then
    wait_duration = (tokens * -1) / bucket.limit
    if wait_duration > max_delay then
        tokens = tokens + 1
        tokens = math.min(tokens, burst)
    end
end

redis.call('hset', key, 'last', t, 'tokens', tokens)
redis.call('expire', key, ttl)

return {tostring(true), tostring(wait_duration),tostring(tokens)}`

var AllowTokenBucketScript = kv.NewScript(AllowTokenBucketRaw)
