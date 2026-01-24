package server

import (
	"context"
	"runtime/debug"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryLoggingInterceptor logs unary RPC calls.
func UnaryLoggingInterceptor(log zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		log.Info().
			Str("method", info.FullMethod).
			Msg("gRPC request started")

		resp, err := handler(ctx, req)

		if err != nil {
			log.Error().
				Err(err).
				Str("method", info.FullMethod).
				Msg("gRPC request failed")
		} else {
			log.Info().
				Str("method", info.FullMethod).
				Msg("gRPC request completed")
		}

		return resp, err
	}
}

// UnaryRecoveryInterceptor recovers from panics in unary handlers.
func UnaryRecoveryInterceptor(log zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Interface("panic", r).
					Str("stack", string(debug.Stack())).
					Str("method", info.FullMethod).
					Msg("gRPC panic recovered")
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// StreamLoggingInterceptor logs streaming RPC calls.
func StreamLoggingInterceptor(log zerolog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		log.Info().
			Str("method", info.FullMethod).
			Bool("client_stream", info.IsClientStream).
			Bool("server_stream", info.IsServerStream).
			Msg("gRPC stream started")

		err := handler(srv, ss)

		if err != nil {
			log.Error().
				Err(err).
				Str("method", info.FullMethod).
				Msg("gRPC stream failed")
		} else {
			log.Info().
				Str("method", info.FullMethod).
				Msg("gRPC stream completed")
		}

		return err
	}
}

// StreamRecoveryInterceptor recovers from panics in stream handlers.
func StreamRecoveryInterceptor(log zerolog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Interface("panic", r).
					Str("stack", string(debug.Stack())).
					Str("method", info.FullMethod).
					Msg("gRPC stream panic recovered")
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}
