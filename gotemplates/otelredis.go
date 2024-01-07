package gotemplates

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

var rdb = GetRedisClient(context.Background())

func GetRedisClient(ctx context.Context) *redis.Client {
	// redis.confで外部接続許可を忘れずに
	addr := fmt.Sprintf("%s:%v",
		GetEnv("REDIS_HOSTNAME", "localhost"),
		GetEnv("REDIS_PORT", "6379"))
	fmt.Printf("redis addr: %s\n", addr)
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	WaitRedis(ctx, rdb)

	if GetEnv("OTEL_SDK_DISABLED", "false") != "true" {
		if err := redisotel.InstrumentTracing(rdb); err != nil {
			panic(err)
		}
	}

	return rdb
}

func WaitRedis(ctx context.Context, rdb *redis.Client) {
	for {
		err := rdb.Ping(ctx).Err()
		if err == nil {
			break
		}
		log.Println(fmt.Errorf("failed to ping Redis on start up. retrying...: %w", err))
		time.Sleep(time.Second * 1)
	}
	log.Println("Succeeded to connect redis!")
}

func ExampleRedis() {
	ctx := context.Background()
	rdb := GetRedisClient(ctx)

	// set
	err := rdb.Set(ctx, "key", "value", 0).Err()
	if err != nil {
		panic(err)
	}

	// get
	val, err := rdb.Get(ctx, "key").Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("key", val)

	val2, err := rdb.Get(ctx, "key2").Result()
	if err == redis.Nil {
		fmt.Println("key2 does not exist")
	} else if err != nil {
		panic(err)
	} else {
		fmt.Println("key2", val2)
	}
}
