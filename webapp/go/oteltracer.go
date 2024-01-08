package main

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// env.shに↓を追記
// OTEL_EXPORTER_OTLP_ENDPOINT=http://monitoring:4318
// OTEL_SERVICE_NAME=isuports
// OTEL_SDK_DISABLED=false

var tracer = otel.Tracer("isucholar")

func initTracer(ctx context.Context) (*sdktrace.TracerProvider, error) {
	if GetEnv("OTEL_SDK_DISABLED", "false") == "true" {
		return nil, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}
