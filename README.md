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
| POST   | /api/v1/videos/upload | Upload video      |
| GET    | /api/v1/videos/{id} | Get video (Protected) |
| GET    | /api/v1/batches/{id} | Get batch status (Protected) |

### Video Upload Example
```bash
curl -X POST http://localhost:8080/api/v1/videos/upload \
  -H "Authorization: Bearer <your_jwt_token>" \
  -F "video=@/path/to/video.mp4"

# Response:
# {
#   "video": { "id": "...", "status": "processing" },
#   "batch_id": "abc-123-xyz"
# }
```

### Video Get Example
```bash
curl -H "Authorization: Bearer <your_jwt_token>" \
  http://localhost:8080/api/v1/videos/{video_id}
```

### Batch Status Example
```bash
curl -H "Authorization: Bearer <your_jwt_token>" \
  http://localhost:8080/api/v1/batches/{batch_id}

# Response:
# {
#   "batch_id": "abc-123-xyz",
#   "status": "completed",
#   "total_jobs": 3,
#   "completed_jobs": 3,
#   "jobs": [
#     { "name": "upload", "status": "completed" },
#     { "name": "transcript", "status": "completed" },
#     { "name": "quiz", "status": "completed" }
#   ]
# }
```

### Batch Immersion Example
```bash
curl -H "Authorization: Bearer <your_jwt_token>" \
  http://localhost:8080/api/v1/batches/{batch_id}/immersions

# Response:
# {
#   "video": { "id": "...", "feature_id": 1, ... },
#   "gist_quiz": { "id": "...", "feature_id": 3, ... },
#   "retell_story": { "id": "...", "feature_id": 8, ... },
#   "batch_id": "abc-123-xyz",
#   "status": "completed"
# }
```

### Video Playlist Example
```bash
curl -H "Authorization: Bearer <your_jwt_token>" \
  "http://localhost:8080/api/v1/videos/playlist?page=1&limit=20&status=new"

# Response:
# {
#   "data": [
#     { 
#       "id": "...", 
#       "feature_id": 1,
#       "metadata": { "status": "new" }
#     },
#     { 
#       "id": "...", 
#       "feature_id": 1,
#       "metadata": { "status": "done" }
#     }
#   ],
#   "total": 2,
#   "page": 1,
#   "limit": 20
# }
```

### Video Action Example
```bash
curl -X POST http://localhost:8080/api/v1/videos/actions \
  -H "Authorization: Bearer <your_jwt_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "video_id": "123e4567-e89b-12d3-a456-426614174000",
    "type": "passed"
  }'

# Response:
# {
#   "status": "success"
# }
```

### Learning Items by Feature Example
```bash
curl -X GET "http://localhost:8080/api/v1/learning-items/feature?feature_id=1&page=1&limit=20" \
  -H "Authorization: Bearer <your_jwt_token>"
  
# Response:
# {
#   "data": [
#     { "id": "...", "feature_id": 1, ... }
#   ],
#   "total": 1,
#   "page": 1,
#   "limit": 20
# }
```

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

## Backup Database (Don't Forget)

```bash
# สั่ง dump ข้อมูลออกมาเป็นไฟล์ SQL เก็บไว้ในเครื่อง VPS
docker exec -t <ชื่อ_db_container> pg_dump -U <db_user> <db_name> > backup_manual_$(date +%F).sql
```

## Restore Database (Don't Forget)

```bash
# สั่ง restore ข้อมูลจากไฟล์ SQL เก็บไว้ในเครื่อง VPS
docker exec -t <ชื่อ_db_container> psql -U <db_user> <db_name> < backup_manual_$(date +%F).sql
```

## License

MIT
