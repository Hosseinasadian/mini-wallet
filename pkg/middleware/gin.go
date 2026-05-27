package middleware

import (
	"github.com/gin-gonic/gin"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	pkgOtel "github.com/hosseinasadian/mini-wallet/pkg/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	"net/http"
	"time"
)

func GinSlogLogger(logger *pkgLogger.Logger, metrics *pkgOtel.HTTPMetrics) gin.HandlerFunc {
	skipPaths := map[string]bool{
		"/live":  true,
		"/ready": true,
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		if skipPaths[c.Request.URL.Path] {
			return
		}

		duration := time.Since(start)

		if metrics != nil {
			metrics.Record(
				c.Request.Context(),
				c.Request.Method,
				c.FullPath(),
				c.Writer.Status(),
				duration,
			)
		}

		span := oteltrace.SpanFromContext(c.Request.Context())
		spanCtx := span.SpanContext()

		fields := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency", time.Since(start).String(),
			"request_id", c.GetHeader("X-Request-ID"),
		}

		if spanCtx.IsValid() {
			fields = append(fields,
				"trace_id", spanCtx.TraceID().String(),
				"span_id", spanCtx.SpanID().String(),
			)
		}

		logger.Info("http request", fields...)
	}
}

func GinSlogRecovery(logger *pkgLogger.Logger) gin.HandlerFunc {
	return gin.RecoveryWithWriter(nil, func(c *gin.Context, err any) {
		logger.Error("panic recovered",
			"error", err,
			"path", c.Request.URL.Path,
		)
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}

func OtelMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		propagator := otel.GetTextMapPropagator()
		ctx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))

		tracer := otel.GetTracerProvider().Tracer(serviceName)
		spanName := c.Request.Method + " " + c.FullPath()
		ctx, span := tracer.Start(ctx, spanName,
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			oteltrace.WithAttributes(
				attribute.String("http.method", c.Request.Method),
				attribute.String("http.target", c.Request.URL.Path),
				attribute.String("http.route", c.FullPath()),
			),
		)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)

		c.Next()

		span.SetAttributes(attribute.Int("http.status_code", c.Writer.Status()))
		if c.Writer.Status() >= 500 {
			span.SetStatus(codes.Error, "server error")
		}
	}
}
