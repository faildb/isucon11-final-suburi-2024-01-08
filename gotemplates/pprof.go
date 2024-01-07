package gotemplates

import (
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"

	"github.com/gorilla/mux"
	"github.com/labstack/echo/v4"
)

// go tool pprof -list main. -cum -seconds 60 http://43.206.3.190/debug/pprof/profile
// https://www.tetsuzawa.com/docs/ISUCON/go/pprof
func PprofEcho(e *echo.Echo) *echo.Echo {
	prefixRouter := e.Group("/debug/pprof")
	{
		prefixRouter.GET("/", echo.WrapHandler(http.HandlerFunc(pprof.Index)))
		prefixRouter.GET("/allocs", echo.WrapHandler(pprof.Handler("allocs")))
		prefixRouter.GET("/block", echo.WrapHandler(pprof.Handler("block")))
		prefixRouter.GET("/cmdline", echo.WrapHandler(http.HandlerFunc(pprof.Cmdline)))
		prefixRouter.GET("/goroutine", echo.WrapHandler(pprof.Handler("goroutine")))
		prefixRouter.GET("/heap", echo.WrapHandler(pprof.Handler("heap")))
		prefixRouter.GET("/mutex", echo.WrapHandler(pprof.Handler("mutex")))
		prefixRouter.GET("/profile", echo.WrapHandler(http.HandlerFunc(pprof.Profile)))
		prefixRouter.POST("/symbol", echo.WrapHandler(http.HandlerFunc(pprof.Symbol)))
		prefixRouter.GET("/symbol", echo.WrapHandler(http.HandlerFunc(pprof.Symbol)))
		prefixRouter.GET("/threadcreate", echo.WrapHandler(pprof.Handler("threadcreate")))
		//prefixRouter.GET("/trace", echo.WrapHandler(pprof.Trace))
	}
	return e
}

func PprofGorilla() {
	r := mux.NewRouter()
	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/allocs", pprof.Handler("allocs").ServeHTTP)
	r.HandleFunc("/debug/pprof/block", pprof.Handler("block").ServeHTTP)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
	r.HandleFunc("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
	r.HandleFunc("/debug/pprof/mutex", pprof.Handler("mutex").ServeHTTP)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.HandleFunc("/debug/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
	//r.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

func PprofStd() {
	http.HandleFunc("/debug/pprof/", pprof.Index)
	http.HandleFunc("/debug/pprof/allocs", pprof.Handler("allocs").ServeHTTP)
	http.HandleFunc("/debug/pprof/block", pprof.Handler("block").ServeHTTP)
	http.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	http.HandleFunc("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
	http.HandleFunc("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
	http.HandleFunc("/debug/pprof/mutex", pprof.Handler("mutex").ServeHTTP)
	http.HandleFunc("/debug/pprof/profile", pprof.Profile)
	http.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	http.HandleFunc("/debug/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
	//http.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
