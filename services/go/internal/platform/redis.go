package platform

import "github.com/redis/go-redis/v9"

func NewRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         Env("REDIS_ADDR", "localhost:6379"),
		PoolSize:     EnvInt("REDIS_POOL", 32),
		MinIdleConns: 4,
	})
}
