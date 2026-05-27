// pkg/otel/http_metrics.go
package otel

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type HTTPMetrics struct {
	requestDuration metric.Float64Histogram
	requestCount    metric.Int64Counter
}

func AddHttpMetrics(mp metric.MeterProvider, meterName string) (*HTTPMetrics, error) {
	if mp == nil {
		return nil, errors.New("metric provider is nil")
	}

	meter := mp.Meter(meterName)

	duration, err := meter.Float64Histogram(
		"http.server.duration",
		metric.WithDescription("HTTP request duration in milliseconds"),
		metric.WithUnit("ms"),
	)

	if err != nil {
		return nil, err
	}

	count, err := meter.Int64Counter(
		"http.server.request.count",
		metric.WithDescription("HTTP request count"),
	)
	if err != nil {
		return nil, err
	}

	return &HTTPMetrics{
		requestDuration: duration,
		requestCount:    count,
	}, nil
}

func (m *HTTPMetrics) Record(ctx context.Context, method, route string, status int, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("http.method", method),
		attribute.String("http.route", route),
		attribute.Int("http.status_code", status),
	}

	m.requestDuration.Record(ctx, float64(duration.Milliseconds()), metric.WithAttributes(attrs...))
	m.requestCount.Add(ctx, 1, metric.WithAttributes(attrs...))
}
