# uwu_service

A production-ready Go service with support for REST API, WebSocket, gRPC, AI services, and cloud integrations.

## Features

- **REST API** - Chi router with middleware
- **WebSocket** - Real-time bidirectional communication
- **gRPC** - High-performance RPC with streaming support
- **AI Services** - OpenAI and Google Gemini integration
- **Cloud Services** - GCS/S3 storage and Pub/Sub messaging
- **Production Ready** - Structured logging, error handling, graceful shutdown

## Quick Start

### Prerequisites

- Go 1.22+
- Docker (optional)
- Protocol Buffers compiler (for regenerating protos)

### Running Locally

1. Copy the environment file:
```bash
cp .env.example .env
```

2. Edit `.env` with your configuration

3. Run the server:
```bash
make run
```

### Using Docker

```bash
make docker-run
```

## Project Structure

```
uwu_service/
├── cmd/server/          # Application entrypoint
├── api/proto/           # Protocol buffer definitions
├── internal/
│   ├── config/          # Configuration management
│   ├── logger/          # Structured logging
│   ├── errors/          # Error handling
│   ├── server/          # HTTP/gRPC/WebSocket servers
│   ├── handler/         # Request handlers
│   ├── middleware/      # HTTP middleware
│   ├── service/         # Business logic
│   ├── repository/      # Data access
│   ├── client/          # External API clients
│   └── pb/              # Generated protobuf code
├── pkg/response/        # API response utilities
└── deployments/docker/  # Docker configurations
```

## API Endpoints

### REST API

| Method | Endpoint          | Description         |
|--------|-------------------|---------------------|
| GET    | /health           | Health check        |
| GET    | /ready            | Readiness check     |
| GET    | /live             | Liveness check      |
| GET    | /api/v1/example   | Get example         |
| POST   | /api/v1/example   | Create example      |
| POST   | /api/v1/ai/chat   | AI chat             |
| POST   | /api/v1/ai/complete | AI completion     |

### WebSocket

Connect to `/ws` for real-time communication.

Message format:
```json
{
  "type": "chat",
  "payload": {
    "message": "Hello!"
  }
}
```

### gRPC

See `api/proto/service.proto` for service definitions.

## Configuration

All configuration is done via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| SERVER_HTTP_PORT | 8080 | HTTP server port |
| SERVER_GRPC_PORT | 50051 | gRPC server port |
| LOG_LEVEL | info | Log level (debug, info, warn, error) |
| OPENAI_API_KEY | - | OpenAI API key |
| GEMINI_API_KEY | - | Google Gemini API key |
| GCP_PROJECT_ID | - | Google Cloud project ID |
| GCS_BUCKET_NAME | - | GCS bucket name |

## Development

### Generate Protobuf

```bash
make proto
```

### Run Tests

```bash
make test
```

### Linting

```bash
make lint
```

## License

MIT
