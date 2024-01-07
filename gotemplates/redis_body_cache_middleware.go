package gotemplates

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
)

// TODO initalizeでredisのinvalidateを忘れないように
func RedisInvalidateAllCache(ctx context.Context, rdb *redis.Client) error {
	if rdb == nil {
		return fmt.Errorf("redis client is nil")
	}
	if err := rdb.FlushAll(ctx).Err(); err != nil {
		return fmt.Errorf("failed to flush all redis cache: %w", err)
	}
	return nil
}

func RedisDeleteKeysMatchingPattern(ctx context.Context, rdb *redis.Client, pattern string) error {
	if rdb == nil {
		return fmt.Errorf("redis client is nil")
	}
	keys, err := rdb.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		fmt.Println("No keys to delete.")
		return nil
	}

	var delCnt int64
	for _, key := range keys {
		delNum, err := rdb.Del(ctx, key).Result()
		if err != nil {
			return fmt.Errorf("failed to delete key: %s, err: %w", key, err)
		}
		delCnt += delNum
	}
	if len(keys) != int(delCnt) {
		return fmt.Errorf("failed to delete all keys, pattern: %s, got keys: %v", pattern, keys)
	}

	return nil
}

func ExampleHandler(c echo.Context) error {
	rres, err := rdb.Get(c.Request().Context(), c.Request().RequestURI).Result()
	if err != nil {
		if err != redis.Nil {
			return fmt.Errorf("error redis.Get: %w", err)
		}
	}
	if err != redis.Nil {
		c.Response().Header().Set("Cache-Status", "HIT")
		return c.JSONBlob(http.StatusOK, []byte(rres))
	}
	c.Response().Header().Set("Cache-Status", "MISS")

	//...
	res := struct {
		Hoge string `json:"hoge"`
	}{Hoge: "hoge"}
	return c.JSON(http.StatusOK, res)
}

type RedisBodyCacheConfig struct {
	Skipper middleware.Skipper
	rdb     *redis.Client
}

type bodyDumpResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

var DefaultRedisBodyCacheConfig = RedisBodyCacheConfig{
	Skipper: RedisBodyCacheDefaultSkipper,
}

func RedisBodyCacheDefaultSkipper(c echo.Context) bool {
	if c.Request().Method != http.MethodGet {
		return true
	}
	return false
}
func RedisBodyCache(rdb *redis.Client) echo.MiddlewareFunc {
	c := DefaultRedisBodyCacheConfig
	c.rdb = rdb
	return RedisBodyCacheWithConfig(c)
}

func RedisBodyCacheWithConfig(config RedisBodyCacheConfig) echo.MiddlewareFunc {
	// Defaults
	if config.rdb == nil {
		panic("echo: body-dump middleware requires a redis client")
	}
	if config.Skipper == nil {
		config.Skipper = DefaultRedisBodyCacheConfig.Skipper
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			if config.Skipper(c) {
				return next(c)
			}

			// Response
			resBody := new(bytes.Buffer)
			mw := io.MultiWriter(c.Response().Writer, resBody)
			writer := &bodyDumpResponseWriter{Writer: mw, ResponseWriter: c.Response().Writer}
			c.Response().Writer = writer

			if err = next(c); err != nil {
				c.Error(err)
			}

			c.Logger().Debugf("caching in RedisBodyCache middleware, url: %s", c.Request().RequestURI)
			err = config.rdb.Set(c.Request().Context(), c.Request().RequestURI, resBody.Bytes(), -1).Err()
			if err != nil {
				c.Logger().Errorf("error in RedisBodyCache middleware, url: %s, error: %s", c.Request().RequestURI, err)
			}
			return
		}
	}
}

func (w *bodyDumpResponseWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
}

func (w *bodyDumpResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w *bodyDumpResponseWriter) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
}

func (w *bodyDumpResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}
