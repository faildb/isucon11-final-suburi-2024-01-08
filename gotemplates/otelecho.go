package gotemplates

import "context"

func main_func() {
	tp, _ := initTracer(context.Background())
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}()
}

// e.Use(otelecho.Middleware("isucholar"))
