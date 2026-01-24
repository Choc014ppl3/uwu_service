package grpc

import (
	"context"
	"io"

	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/pb"
	"github.com/windfall/uwu_service/internal/service"
)

// Handler implements the gRPC service.
type Handler struct {
	pb.UnimplementedUwuServiceServer
	log            zerolog.Logger
	aiService      *service.AIService
	exampleService *service.ExampleService
}

// NewHandler creates a new gRPC handler.
func NewHandler(
	log zerolog.Logger,
	aiService *service.AIService,
	exampleService *service.ExampleService,
) *Handler {
	return &Handler{
		log:            log,
		aiService:      aiService,
		exampleService: exampleService,
	}
}

// GetExample implements the GetExample RPC.
func (h *Handler) GetExample(ctx context.Context, req *pb.GetExampleRequest) (*pb.ExampleResponse, error) {
	h.log.Info().Str("id", req.Id).Msg("GetExample called")

	result, err := h.exampleService.GetExample(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &pb.ExampleResponse{
		Id:          result.ID,
		Name:        result.Name,
		Description: result.Description,
	}, nil
}

// CreateExample implements the CreateExample RPC.
func (h *Handler) CreateExample(ctx context.Context, req *pb.CreateExampleRequest) (*pb.ExampleResponse, error) {
	h.log.Info().Str("name", req.Name).Msg("CreateExample called")

	result, err := h.exampleService.CreateExample(ctx, req.Name, req.Description)
	if err != nil {
		return nil, err
	}

	return &pb.ExampleResponse{
		Id:          result.ID,
		Name:        result.Name,
		Description: result.Description,
	}, nil
}

// Chat implements the Chat RPC.
func (h *Handler) Chat(ctx context.Context, req *pb.ChatRequest) (*pb.ChatResponse, error) {
	h.log.Info().Str("provider", req.Provider).Msg("Chat called")

	result, err := h.aiService.Chat(ctx, req.Message, req.Provider)
	if err != nil {
		return nil, err
	}

	return &pb.ChatResponse{
		Response: result,
		Provider: req.Provider,
	}, nil
}

// StreamChat implements the streaming Chat RPC.
func (h *Handler) StreamChat(stream pb.UwuService_StreamChatServer) error {
	h.log.Info().Msg("StreamChat started")

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		h.log.Debug().Str("message", req.Message).Msg("Received chat message")

		// Process the message
		result, err := h.aiService.Chat(stream.Context(), req.Message, req.Provider)
		if err != nil {
			return err
		}

		// Send response
		if err := stream.Send(&pb.ChatResponse{
			Response: result,
			Provider: req.Provider,
		}); err != nil {
			return err
		}
	}
}

// HealthCheck implements the health check RPC.
func (h *Handler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Status:  pb.HealthCheckResponse_SERVING,
		Service: "uwu_service",
	}, nil
}
