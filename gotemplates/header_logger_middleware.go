package gotemplates

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func HeaderLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Logger().Debug(c.Request().Header)
			return next(c)
		}
	}
}

func LoggerWithIfNoneMatchHeader() echo.MiddlewareFunc {
	cfg := middleware.DefaultLoggerConfig
	cfg.Format = `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}",` +
		`"host":"${host}","method":"${method}","uri":"${uri}","user_agent":"${user_agent}","if_none_match":"${header:If-None-Match}",` +
		`"status":${status},"error":"${error}","latency":${latency},"latency_human":"${latency_human}"` +
		`,"bytes_in":${bytes_in},"bytes_out":${bytes_out}}` + "\n"
	return middleware.LoggerWithConfig(cfg)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Logger().Debug(c.Request().Header)
			return next(c)
		}
	}
}
