package server

import (
	"net"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/windfall/uwu_service/internal/config"
	grpchandler "github.com/windfall/uwu_service/internal/handler/grpc"
	"github.com/windfall/uwu_service/internal/pb"
)

// GRPCServer represents the gRPC server.
type GRPCServer struct {
	server *grpc.Server
	addr   string
	log    zerolog.Logger
}

// NewGRPCServer creates a new gRPC server.
func NewGRPCServer(
	cfg *config.Config,
	log zerolog.Logger,
	handler *grpchandler.Handler,
) *GRPCServer {
	// Create gRPC server with interceptors
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			UnaryLoggingInterceptor(log),
			UnaryRecoveryInterceptor(log),
		),
		grpc.ChainStreamInterceptor(
			StreamLoggingInterceptor(log),
			StreamRecoveryInterceptor(log),
		),
	)

	// Register services
	pb.RegisterUwuServiceServer(server, handler)

	// Enable reflection for development
	if cfg.IsDevelopment() {
		reflection.Register(server)
	}

	return &GRPCServer{
		server: server,
		addr:   cfg.GRPCAddress(),
		log:    log,
	}
}

// Start starts the gRPC server.
func (s *GRPCServer) Start() error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	s.log.Info().Str("addr", s.addr).Msg("Starting gRPC server")
	if err := s.server.Serve(lis); err != nil {
		return err
	}
	return nil
}

// GracefulStop gracefully stops the gRPC server.
func (s *GRPCServer) GracefulStop() {
	s.log.Info().Msg("Shutting down gRPC server")
	s.server.GracefulStop()
}
