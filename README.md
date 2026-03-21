# uwu_service

A production-ready Go backend service for an AI-powered Language Learning Platform. It provides video immersion features, AI roleplay dialogues, pronunciation assessments, and real-time chat practice.

## Core Features

- **REST API** - Clean architecture using Chi router with JWT authentication and middleware.
- **AI Services** - Deep integrations with:
  - Azure OpenAI (GPT-5 Nano/ChatGPT) for dialogue generation and chat scenarios.
  - Azure Cognitive Services (Whisper and Speech-to-Text) for pronunciation assessment.
  - Google Gemini for dialogue scene image generation.
- **Asynchronous Processing** - Background job queues using custom Goroutine workers for media processing, transcript generation, and quiz creation.
- **State Management** - Real-time batch job tracking using Redis.
- **Cloud Storage** - Cloudflare R2 (S3-compatible) integration for storing generated audio, images, and user uploads.
- **Production Ready** - Structured JSON logging (`log/slog`), graceful shutdown, clean domain-driven architecture, and PostgreSQL for persistent data.

## Quick Start

### Prerequisites

- Go 1.22+
- PostgreSQL
- Redis
- API Keys for Azure (OpenAI & Speech) and Google Cloud (Gemini)

### Running Locally

1. Copy the environment file:
```bash
cp .env.example .env
```

2. Edit `.env` with your database connections and API keys.

3. Run the server:
```bash
make run
```

### Using Docker

```bash
make docker-run
```

## Project Structure

```text
uwu_service/
├── cmd/server/          # Application entrypoint
├── internal/
│   ├── config/          # Environment configuration management
│   ├── domain/          # Core business domains (auth, dialog, profile, video)
│   ├── infra/           # External clients (Azure, Gemini), HTTP server, Middleware
│   └── pb/              # (Reserved for future protobuf code)
├── pkg/
│   ├── errors/          # Custom application error handling
│   ├── logger/          # Structured logging setup
│   └── response/        # Standardized JSON response utilities
└── deployments/docker/  # Docker and compose configurations
```

## API Endpoints

### 1. Health checks (Public)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET    | `/health` | Service health status |

### 2. Authentication (Public)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST   | `/api/v1/auth/register` | Register a new user |
| POST   | `/api/v1/auth/login` | Login and get JWT token |

### 3. Dialogs (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET    | `/api/v1/dialogs/contents` | List paginated dialog contents |
| POST   | `/api/v1/dialogs/generate` | Generate dialog content (Async) |
| GET    | `/api/v1/dialogs/{dialogID}/details`| Get dialog details/results |
| POST   | `/api/v1/dialogs/{dialogID}/start-speech` | Start dialogue speech practice session|
| POST   | `/api/v1/dialogs/{dialogID}/actions/{actionID}/submit-speech` | Submit spoken audio for scoring |
| POST   | `/api/v1/dialogs/{dialogID}/start-chat` | Start dialogue chat session |
| POST   | `/api/v1/dialogs/{dialogID}/actions/{actionID}/submit-chat` | Send message to AI chat partner |
| POST   | `/api/v1/dialogs/{dialogID}/toggle-saved` | Save or unsave dialog |
| POST   | `/api/v1/dialogs/generate-image` | Generate image from prompt |

### 4. Videos (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET    | `/api/v1/videos/contents` | List paginated video contents |
| POST   | `/api/v1/videos/upload` | Upload video and thumbnail (Async) |
| GET    | `/api/v1/videos/{videoID}/details` | Get video details/processing status |
| POST   | `/api/v1/videos/{videoID}/start-quiz` | Start video quiz session |
| POST   | `/api/v1/videos/{videoID}/toggle-transcript` | Toggle transcript visibility |
| POST   | `/api/v1/videos/{videoID}/toggle-saved` | Save or unsave video |

### 5. Profile (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET    | `/api/v1/profile` | Get user profile stats |

---

## cURL Examples

### Dialog Generation

```bash
# 1. Start generation
curl -X POST http://localhost:8080/api/v1/dialogs/generate \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "Ordering Coffee",
    "description": "At a busy coffee shop",
    "language": "spanish",
    "level": "intermediate"
  }'

# 2. Get status/details
curl -H "Authorization: Bearer <jwt>" \
  http://localhost:8080/api/v1/dialogs/{dialogID}/details
```

### Video Upload

```bash
curl -X POST http://localhost:8080/api/v1/videos/upload \
  -H "Authorization: Bearer <jwt>" \
  -H "Language: english" \
  -F "video=@/path/to/video.mp4" \
  -F "thumbnail=@/path/to/thumb.jpg"
```

### Submit Speech (Multipart)

```bash
curl -X POST http://localhost:8080/api/v1/dialogs/{dialogID}/actions/{actionID}/submit-speech \
  -H "Authorization: Bearer <jwt>" \
  -F "audio=@/path/to/audio.wav" \
  -F "original_text=Hola, quisiera un café." \
  -F "script_index=0" \
  -F "language=es-ES"
```

### Submit Chat (JSON)

```bash
curl -X POST http://localhost:8080/api/v1/dialogs/{dialogID}/actions/{actionID}/submit-chat \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "I would like to order a latte."
  }'
```


## Development

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
