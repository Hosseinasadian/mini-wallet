package interceptors

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type GRPCMetrics struct {
	requestDuration metric.Float64Histogram
	requestCount    metric.Int64Counter
}

func NewGRPCMetrics(mp metric.MeterProvider, meterName string) (*GRPCMetrics, error) {
	meter := mp.Meter(meterName)

	duration, err := meter.Float64Histogram(
		"grpc.duration",
		metric.WithDescription("gRPC request duration in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	count, err := meter.Int64Counter(
		"grpc.request.count",
		metric.WithDescription("gRPC request count"),
	)
	if err != nil {
		return nil, err
	}

	return &GRPCMetrics{
		requestDuration: duration,
		requestCount:    count,
	}, nil
}

func (m *GRPCMetrics) record(ctx context.Context, serviceName, method, status string, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("rpc.service", serviceName),
		attribute.String("rpc.method", method),
		attribute.String("rpc.status", status),
	}
	m.requestDuration.Record(ctx, float64(duration.Milliseconds()), metric.WithAttributes(attrs...))
	m.requestCount.Add(ctx, 1, metric.WithAttributes(attrs...))
}

type Interceptors struct {
	serviceName string
	logger      *pkgLogger.Logger
	metrics     *GRPCMetrics
}

func New(serviceName string, logger *pkgLogger.Logger, metrics *GRPCMetrics) *Interceptors {
	return &Interceptors{
		serviceName: serviceName,
		logger:      logger,
		metrics:     metrics,
	}
}

func (i *Interceptors) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		p, _ := peer.FromContext(ctx)
		peerAddr := ""
		if p != nil {
			peerAddr = p.Addr.String()
		}

		tracer := otel.Tracer(i.serviceName)
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", i.serviceName),
				attribute.String("rpc.method", info.FullMethod),
				attribute.String("peer.address", peerAddr),
			),
		)
		defer span.End()

		var (
			resp interface{}
			err  error
		)

		defer func() {
			if r := recover(); r != nil {
				i.logger.Error("panic in gRPC handler",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				span.RecordError(fmt.Errorf("panic: %v", r))
				span.SetStatus(codes.Error, "panic")
				err = status.Errorf(grpcCodes.Internal, "internal server error")
			}
		}()

		resp, err = handler(ctx, req)

		duration := time.Since(start)
		st, _ := status.FromError(err)
		statusCode := st.Code().String()

		if i.metrics != nil {
			i.metrics.record(ctx, i.serviceName, info.FullMethod, statusCode, duration)
		}

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			i.logger.Error("gRPC request failed",
				"method", info.FullMethod,
				"duration", duration,
				"status", statusCode,
				"error", err,
			)
		} else {
			span.SetStatus(codes.Ok, "success")
		}

		return resp, err
	}
}

func (i *Interceptors) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		ctx := ss.Context()

		p, _ := peer.FromContext(ctx)
		peerAddr := ""
		if p != nil {
			peerAddr = p.Addr.String()
		}

		tracer := otel.Tracer(i.serviceName)
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", i.serviceName),
				attribute.String("rpc.method", info.FullMethod),
				attribute.String("peer.address", peerAddr),
				attribute.Bool("stream", true),
			),
		)
		defer span.End()

		var err error
		defer func() {
			if r := recover(); r != nil {
				i.logger.Error("panic in gRPC stream handler",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				span.RecordError(fmt.Errorf("panic: %v", r))
				span.SetStatus(codes.Error, "panic")
				err = status.Errorf(grpcCodes.Internal, "internal server error")
			}
		}()

		err = handler(srv, &wrappedServerStream{ServerStream: ss, ctx: ctx})

		duration := time.Since(start)
		st, _ := status.FromError(err)
		statusCode := st.Code().String()

		if i.metrics != nil {
			i.metrics.record(ctx, i.serviceName, info.FullMethod, statusCode, duration)
		}

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			i.logger.Error("gRPC stream failed",
				"method", info.FullMethod,
				"duration", duration,
				"error", err,
			)
		} else {
			span.SetStatus(codes.Ok, "success")
			i.logger.Info("gRPC stream completed",
				"method", info.FullMethod,
				"duration", duration,
			)
		}

		return err
	}
}

func (i *Interceptors) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()

		tracer := otel.Tracer(i.serviceName)
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", i.serviceName),
				attribute.String("rpc.method", method),
			),
		)
		defer span.End()

		err := invoker(ctx, method, req, reply, cc, opts...)

		duration := time.Since(start)
		st, _ := status.FromError(err)
		statusCode := st.Code().String()

		if i.metrics != nil {
			i.metrics.record(ctx, i.serviceName, method, statusCode, duration)
		}

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			i.logger.Error("gRPC client request failed",
				"method", method,
				"duration", duration,
				"error", err,
			)
		} else {
			span.SetStatus(codes.Ok, "success")
		}

		return err
	}
}

func (i *Interceptors) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		start := time.Now()

		tracer := otel.Tracer(i.serviceName)
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", i.serviceName),
				attribute.String("rpc.method", method),
				attribute.Bool("stream", true),
			),
		)

		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			i.logger.Error("gRPC client stream failed",
				"method", method,
				"duration", time.Since(start),
				"error", err,
			)
			return nil, err
		}

		return &wrappedClientStream{
			ClientStream: clientStream,
			span:         span,
			method:       method,
			start:        start,
			interceptors: i,
			ctx:          ctx,
		}, nil
	}
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

type wrappedClientStream struct {
	grpc.ClientStream
	span         trace.Span
	method       string
	start        time.Time
	interceptors *Interceptors
	ctx          context.Context
}

func (w *wrappedClientStream) RecvMsg(m interface{}) error {
	err := w.ClientStream.RecvMsg(m)
	if err != nil {
		st, _ := status.FromError(err)
		statusCode := st.Code().String()

		if w.interceptors.metrics != nil {
			w.interceptors.metrics.record(w.ctx, w.interceptors.serviceName, w.method, statusCode, time.Since(w.start))
		}

		w.span.RecordError(err)
		w.span.SetStatus(codes.Error, err.Error())
		w.span.End()
	}
	return err
}

func (w *wrappedClientStream) CloseSend() error {
	err := w.ClientStream.CloseSend()
	if err == nil {
		if w.interceptors.metrics != nil {
			w.interceptors.metrics.record(w.ctx, w.interceptors.serviceName, w.method, "OK", time.Since(w.start))
		}
		w.span.SetStatus(codes.Ok, "success")
		w.span.End()
	}
	return err
}
