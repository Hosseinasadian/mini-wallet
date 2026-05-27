package middleware

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hosseinasadian/mini-wallet/pkg/logger"
	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const (
	CorrelationID contextKey = "correlation_id"
	IPAddress     contextKey = "ip_address"
	UserAgent     contextKey = "user_agent"
	Path          contextKey = "path"
)

func DeviceContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		userAgent := c.GetHeader("User-Agent")
		ipStr := c.ClientIP()
		path := c.Request.URL.Path
		correlationID := uuid.New().String()

		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, UserAgent, userAgent)
		ctx = context.WithValue(ctx, IPAddress, ipStr)
		ctx = context.WithValue(ctx, Path, path)
		ctx = context.WithValue(ctx, CorrelationID, correlationID)
		c.Request = c.Request.WithContext(ctx)

		c.Header("X-Request-ID", correlationID)

		c.Next()
	}
}

func GetCorrelationID(ctx context.Context) string {
	if v := ctx.Value(CorrelationID); v != nil {
		return v.(string)
	}
	return ""
}

func GetIPAddress(ctx context.Context) string {
	if v := ctx.Value(IPAddress); v != nil {
		return v.(string)
	}
	return ""
}

func GetUserAgent(ctx context.Context) string {
	if v := ctx.Value(UserAgent); v != nil {
		return v.(string)
	}
	return ""
}

func GetPath(ctx context.Context) string {
	if v := ctx.Value(Path); v != nil {
		return v.(string)
	}
	return ""
}

func getLoggerArguments(ctx context.Context) []any {
	args := []any{
		"request_id", GetCorrelationID(ctx),
		"ip_address", GetIPAddress(ctx),
		"path", GetPath(ctx),
	}

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		args = append(args,
			"trace_id", span.SpanContext().TraceID().String(),
			"span_id", span.SpanContext().SpanID().String(),
		)
	}

	return args
}

func GetLoggerContext(ctx context.Context, defaultLogger *logger.Logger) *logger.Logger {
	args := getLoggerArguments(ctx)
	return defaultLogger.With(args...)
}
