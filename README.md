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

## Standardized Attempt History

- Refactored `StartQuiz` and `StartRetell` to return the full history of previous tries in a unified `attempts` field.
- Standardized the database metadata property name to `attempts` for both gist quiz and retell story actions.
- Implemented backward compatibility to handle legacy `quiz_attempts` and `retell_attempts` fields during the transition.

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
| POST   | `/api/v1/dialogs/{dialogID}/submit-speech` | Submit spoken audio for scoring |
| POST   | `/api/v1/dialogs/{dialogID}/start-chat` | Start dialogue chat session |
| POST   | `/api/v1/dialogs/{dialogID}/submit-chat` | Send message to AI chat partner |
| POST   | `/api/v1/dialogs/{dialogID}/toggle-saved` | Save or unsave dialog |

### 4. Videos (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET    | `/api/v1/videos/contents` | List paginated video contents |
| POST   | `/api/v1/videos/upload` | Upload video and thumbnail (Async) |
| GET    | `/api/v1/videos/{videoID}/details` | Get video details/processing status |
| POST   | `/api/v1/videos/{videoID}/start-quiz` | Start gist quiz session |
| POST   | `/api/v1/videos/{videoID}/start-retell` | Start retell story session |
| POST   | `/api/v1/videos/{videoID}/submit-quiz` | Submit gist quiz answers |
| POST   | `/api/v1/videos/{videoID}/submit-retell` | Submit retell story audio |
| POST   | `/api/v1/videos/{videoID}/toggle-transcript` | Toggle transcript visibility |
| POST   | `/api/v1/videos/{videoID}/toggle-saved` | Save or unsave video |

### 5. Profile (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET    | `/api/v1/profile` | Get user profile stats |

---

## cURL Examples

### 1. Health Checks

```bash
curl -X GET http://localhost:8080/health
```

### 2. Authentication

**Register User:**
```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123",
    "display_name": "testuser",
    "avatar_url": "https://api.dicebear.com/7.x/initials/svg?seed=testuser"
  }'
```

**Login:**
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'
```

### 3. Dialogs

**List Dialog Contents:**
```bash
curl -X GET "http://localhost:8080/api/v1/dialogs/contents?page=1&page_size=10" \
  -H "Authorization: Bearer <jwt>"
```

**Generate Dialog (Async):**
```bash
curl -X POST http://localhost:8080/api/v1/dialogs/generate \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "Ordering Coffee",
    "description": "At a busy coffee shop",
    "language": "spanish",
    "level": "intermediate",
    "tags": ["travel", "food"]
  }'
```

**Get Dialog Details:**
```bash
curl -X GET http://localhost:8080/api/v1/dialogs/{dialogID}/details \
  -H "Authorization: Bearer <jwt>"
```

**Start Dialogue Speech Practice:**
```bash
curl -X POST http://localhost:8080/api/v1/dialogs/{dialogID}/start-speech \
  -H "Authorization: Bearer <jwt>"
```
Example Response:
```json
{
  "action_id": "uuid",
  "dialog_id": "uuid",
  "user_id": "uuid",
  "metadata": {
    "situation_text": "...",
    "situation_audio_url": "...",
    "scripts": [
      { "speaker": "AI", "text": "...", "audio_url": "..." },
      { "speaker": "User", "text": "..." }
    ],
    "attempts": []
  }
}
```

**Submit Speech (Multipart Audio Scoring):**
```bash
curl -X POST http://localhost:8080/api/v1/dialogs/{dialogID}/submit-speech \
  -H "Authorization: Bearer <jwt>" \
  -H "Language: spanish" \
  -F "audio=@/path/to/audio.wav" \
  -F "reference_text=Hola, quisiera un café." \
  -F "script_index=0"
```
Example Response:
```json
{
  "situation_text": "...",
  "situation_audio_url": "...",
  "scripts": [
    {
      "speaker": "User",
      "text": "Hola, quisiera un café.",
      "evaluation": {
        "AccuracyScore": 100.0,
        "FluencyScore": 100.0,
        "PronScore": 100.0
      }
    }
  ],
  "attempts": []
}
```

**Start Dialogue Chat Practice:**
```bash
curl -X POST http://localhost:8080/api/v1/dialogs/{dialogID}/start-chat \
  -H "Authorization: Bearer <jwt>"
```

**Submit Chat (AI Partner Request):**
```bash
curl -X POST http://localhost:8080/api/v1/dialogs/{dialogID}/submit-chat \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "I would like to order a latte."
  }'
```

**Toggle Saved Status:**
```bash
curl -X POST http://localhost:8080/api/v1/dialogs/{dialogID}/toggle-saved \
  -H "Authorization: Bearer <jwt>"
```


### 4. Videos

**List Video Contents:**
```bash
curl -X GET "http://localhost:8080/api/v1/videos/contents?page=1&page_size=10" \
  -H "Authorization: Bearer <jwt>"
```

**Upload Video (Async):**
```bash
curl -X POST http://localhost:8080/api/v1/videos/upload \
  -H "Authorization: Bearer <jwt>" \
  -H "Language: english" \
  -F "video=@/path/to/video.mp4" \
  -F "thumbnail=@/path/to/thumb.jpg"
```

**Get Video Details:**
```bash
curl -H "Authorization: Bearer <jwt>" \
  http://localhost:8080/api/v1/videos/{videoID}/details
```

**Start Video Quiz:**
(Initializes a gist quiz session and returns the questions)
```bash
curl -X POST http://localhost:8080/api/v1/videos/{videoID}/start-quiz \
  -H "Authorization: Bearer <jwt>"
```
Example Response:
```json
  {
    "quiz_questions": [...],
    "attempts": []
  }
```

**Start Retell Story:**
(Initializes a retell story session and returns the key points)
```bash
curl -X POST http://localhost:8080/api/v1/videos/{videoID}/start-retell \
  -H "Authorization: Bearer <jwt>"
```
Example Response:
```json
  {
    "retell_story": {...},
    "attempts": []
  }
```

**Submit Gist Quiz:**
(Submits multiple-choice/ordering answers as JSON)
```bash
curl -X POST http://localhost:8080/api/v1/videos/{videoID}/submit-quiz \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{
    "answers": [
      {"quiz_id": 1, "type": "single_choice", "option_ids": ["A"]},
      {"quiz_id": 2, "type": "ordering", "order": ["C", "A", "B"]}
    ]
  }'
```

**Submit Retell Story:**
(Submits recorded audio file for scoring)
```bash
curl -X POST http://localhost:8080/api/v1/videos/{videoID}/submit-retell \
  -H "Authorization: Bearer <jwt>" \
  -F "audio=@/path/to/retell_voice.wav"
```

**Toggle Transcript Visibility:**
```bash
curl -X POST http://localhost:8080/api/v1/videos/{videoID}/toggle-transcript \
  -H "Authorization: Bearer <jwt>"
```

**Toggle Saved Status:**
```bash
curl -X POST http://localhost:8080/api/v1/videos/{videoID}/toggle-saved \
  -H "Authorization: Bearer <jwt>"
```

### 5. Profile

**Get Profile Stats:**
```bash
curl -X GET http://localhost:8080/api/v1/profile \
  -H "Authorization: Bearer <jwt>"
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

---

## AI-Powered Features

This service utilizes various third-party AI models to power its learning features, mapped by endpoint below:

### 1. Dialogs

#### **POST /api/v1/dialogs/generate**
(Async background processing)
- **Azure OpenAI (GPT-5 Nano)**: Generates dialog scenarios, character scripts, and learning objectives.
- **Vertex AI (Imagen 3 Flash)**: Generates a thematic background image based on the scenario.
- **Azure AI Speech (TTS)**: Synthesizes high-quality audio for AI characters and situational openings.

#### **POST /api/v1/dialogs/{dialogID}/submit-speech**
- **Azure AI Speech (Pronunciation Assessment)**: Evaluates user audio for accuracy, fluency, prosody, and completeness.

#### **POST /api/v1/dialogs/{dialogID}/submit-chat**
- **Azure OpenAI (GPT-5 Nano)**: Generates interactive AI partner replies, provides contextual feedback, and tracks objective completion.

### 2. Videos

#### **POST /api/v1/videos/upload**
(Async background processing)
- **Azure Whisper**: Transcribes the source video audio into text.
- **Azure OpenAI (GPT-5 Nano)**: Analyzes the transcript to generate metadata (topic, level, tags), gist quizzes, and retell key points.

#### **POST /api/v1/videos/{videoID}/submit-retell**
- **Azure Whisper**: Transcribes the user's spoken retell attempt.
- **Azure OpenAI (GPT-5 Nano)**: Evaluates the user's transcript for accuracy against the source material's key points.
