# Claude Orchestrator

A Go + Temporal-based orchestration system for coordinating multiple Claude AI agents to accomplish complex software development tasks. Uses a "Queen" (Orchestrator) pattern to decompose tasks, delegate to specialized agents, and aggregate results.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           TEMPORAL CLUSTER                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                    ORCHESTRATOR WORKFLOW (Queen)                      │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                   │   │
│  │  │ Task Queue  │  │ Agent Pool  │  │  Memory     │                   │   │
│  │  │ Manager     │  │ Coordinator │  │  Aggregator │                   │   │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                   │   │
│  └─────────┼────────────────┼────────────────┼──────────────────────────┘   │
│            │                │                │                               │
│            ▼                ▼                ▼                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                      CHILD WORKFLOWS (Agents)                        │    │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌────────────┐  │    │
│  │  │  Researcher  │ │    Coder     │ │   Reviewer   │ │  Planner   │  │    │
│  │  │    Agent     │ │    Agent     │ │    Agent     │ │   Agent    │  │    │
│  │  └──────────────┘ └──────────────┘ └──────────────┘ └────────────┘  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Features

### Core Features
- **Task Decomposition**: Automatically breaks down complex tasks into subtasks
- **Specialized Agents**: Different agent types for planning, research, coding, reviewing, and execution
- **Parallel Execution**: Execute independent tasks concurrently
- **Memory System**: Shared memory between agents with namespacing
- **Fault Tolerance**: Built-in retries and durability via Temporal
- **Multiple Interfaces**: CLI, REST API, and direct Temporal workflow execution

### Advanced Features (Configurable)
- **Document Analysis**: Read and analyze project documents to extract requirements
- **Requirements Tracking**: Track, manage, and link requirements to implementations
- **Framework Learning**: Learn any framework and feed knowledge to Claude for development
- **Cloud Storage**: Store and serve files via S3, MinIO, GCS, or local filesystem with signed URLs
- **RAG (Retrieval-Augmented Generation)**: Index documents with embeddings for semantic search

## Prerequisites

- Go 1.21 or later
- Temporal server (can run locally with Docker)
- Claude access via one of:
  - **API Provider**: Anthropic API key (pay-per-token credits)
  - **Claude Code Provider**: Claude Code CLI with subscription
- PostgreSQL (optional, for persistent memory)

## Installation

```bash
# Clone the repository
git clone https://github.com/anthropics/claude-orchestrator.git
cd claude-orchestrator

# Install dependencies
go mod tidy

# Build the binaries
make build

# Or build individually
go build -o bin/worker ./cmd/worker
go build -o bin/cli ./cmd/cli
go build -o bin/api ./cmd/api
```

## Quick Start

### 1. Start Temporal Server

Using Docker Compose:
```bash
curl -O https://raw.githubusercontent.com/temporalio/docker-compose/main/docker-compose.yml
docker-compose up -d
```

Or using Temporal CLI:
```bash
temporal server start-dev
```

### 2. Start the Worker

**Option A: Using API Provider (pay-per-token)**
```bash
export CLAUDE_PROVIDER=api
export ANTHROPIC_API_KEY=your-api-key
export TEMPORAL_ADDRESS=localhost:7233
export WORKING_DIR=$(pwd)

./bin/worker
```

**Option B: Using Claude Code Provider (subscription-based)**
```bash
# Install and authenticate Claude Code CLI first
npm install -g @anthropic-ai/claude-code
claude login

# Then start the worker
export CLAUDE_PROVIDER=claude_code
export TEMPORAL_ADDRESS=localhost:7233
export WORKING_DIR=$(pwd)

./bin/worker
```

### 3. Run a Task

Using the CLI:
```bash
./bin/cli run \
  --title "Build REST API" \
  --description "Create a REST API with user authentication using Go and PostgreSQL" \
  --wait
```

Or using the API:
```bash
# Start the API server
./bin/api

# Create a workflow
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Build REST API",
    "description": "Create a REST API with user authentication",
    "parallel": true,
    "review": true
  }'
```

## Claude Code Integration

The orchestrator supports two Claude providers:

| Provider | Billing Model | Authentication | Docker Support |
|----------|--------------|----------------|----------------|
| `api` | Pay-per-token credits | `ANTHROPIC_API_KEY` | Full support |
| `claude_code` | Claude subscription | `claude login` | Requires auth mount |

### Using Claude Code (Subscription)

Claude Code lets you use your Claude subscription instead of API credits:

```bash
# 1. Install Claude Code CLI
npm install -g @anthropic-ai/claude-code

# 2. Authenticate (opens browser)
claude login

# 3. Run worker with Claude Code provider
export CLAUDE_PROVIDER=claude_code
./bin/worker
```

### Docker with Claude Code

For Docker deployments with Claude Code, mount your auth credentials:

```bash
# Build Claude Code-enabled image
docker build -f Dockerfile.claudecode -t claude-orchestrator-claudecode .

# Run with auth mount
docker run -v $HOME/.claude:/home/orchestrator/.claude:ro \
  -e CLAUDE_PROVIDER=claude_code \
  claude-orchestrator-claudecode
```

Or use the dedicated docker-compose file:

```bash
docker-compose -f docker-compose.claude-code.yml up -d
```

### Provider Auto-Detection

If `CLAUDE_PROVIDER` is not set:
- Uses `api` if `ANTHROPIC_API_KEY` is set
- Uses `claude_code` if no API key is present

For detailed documentation, see [docs/CLAUDE_CODE_INTEGRATION.md](docs/CLAUDE_CODE_INTEGRATION.md).

## Configuration

### Configuration File

Create a `config.yaml` file:

```yaml
temporal:
  address: "localhost:7233"
  namespace: "default"
  task_queue: "claude-orchestrator"

claude:
  provider: "api"  # or "claude_code" for subscription
  model: "claude-sonnet-4-20250514"
  max_tokens: 4096
  # Claude Code specific settings (only used when provider is "claude_code")
  working_dir: "."
  allowed_tools:
    - "Read"
    - "Write"
    - "Bash"
    - "Glob"
    - "Grep"

workflow:
  max_agents: 10
  task_timeout_seconds: 300
  review_enabled: true
  parallel_execution: true

memory:
  type: "inmemory"  # or "postgres"
  postgres:
    host: "localhost"
    port: 5432
    database: "claude_orchestrator"
    user: "postgres"
    password: "postgres"
    ssl_mode: "disable"

features:
  document_analysis:
    enabled: true
    max_document_size_mb: 10
    supported_formats:
      - ".md"
      - ".txt"
      - ".pdf"
      - ".docx"
  requirements_tracking:
    enabled: true
    auto_link_implementations: true
  framework_learning:
    enabled: true
    cache_ttl_hours: 24
  code_review: true
  parallel_execution: true

agents:
  orchestrator:
    enabled: true
    max_tokens: 4096
  planner:
    enabled: true
    max_tokens: 4096
  researcher:
    enabled: true
    max_tokens: 4096
  coder:
    enabled: true
    max_tokens: 8192
  reviewer:
    enabled: true
    max_tokens: 4096
  executor:
    enabled: true
    max_tokens: 4096
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CLAUDE_PROVIDER` | Claude provider: `api` or `claude_code` | `api` |
| `ANTHROPIC_API_KEY` | Anthropic API key (required for `api` provider) | - |
| `CLAUDE_MODEL` | Claude model to use | `claude-sonnet-4-20250514` |
| `CLAUDE_ALLOWED_TOOLS` | Allowed tools for Claude Code (comma-separated) | `Read,Write,Bash,Glob,Grep` |
| `TEMPORAL_ADDRESS` | Temporal server address | `localhost:7233` |
| `TEMPORAL_NAMESPACE` | Temporal namespace | `default` |
| `TASK_QUEUE` | Temporal task queue name | `claude-orchestrator` |
| `WORKING_DIR` | Working directory for file operations | `.` |
| `PORT` | API server port | `8080` |
| `CONFIG_PATH` | Path to configuration file | - |

### Feature Toggle Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `FEATURE_DOC_ANALYSIS` | Enable document analysis | `true` |
| `FEATURE_REQ_TRACKING` | Enable requirements tracking | `true` |
| `FEATURE_FRAMEWORK_LEARNING` | Enable framework learning | `true` |
| `FEATURE_CODE_REVIEW` | Enable code review | `true` |
| `FEATURE_PARALLEL_EXECUTION` | Enable parallel execution | `true` |

## Document Analysis

The document analysis feature allows Claude to read and analyze project documents to extract structured requirements.

### Supported Document Types

- **Requirements Documents**: Specifications, PRDs, user stories
- **Design Documents**: Architecture docs, API specs
- **General Documents**: README, guides, technical docs

### Usage

```go
// Using the document activities
documentActivities.ReadDocument(ctx, ReadDocumentRequest{
    Path:         "/path/to/requirements.md",
    DocumentType: DocumentTypeRequirements,
    ExtractItems: []string{"requirements", "features", "constraints"},
})

// Analyze multiple documents
documentActivities.AnalyzeRequirements(ctx, AnalyzeRequirementsRequest{
    Paths:       []string{"/docs/spec.md", "/docs/requirements.txt"},
    ProjectName: "MyProject",
})
```

### Extracted Data

- **Requirements**: Functional, non-functional, technical requirements
- **Features**: Product features and capabilities
- **Constraints**: Technical and business constraints
- **Dependencies**: External dependencies
- **Technologies**: Identified tech stack

## Requirements Tracking

Track requirements throughout the development lifecycle with status updates, implementation linking, and history.

### Requirement Types

- `functional`: Feature requirements
- `non_functional`: Performance, security, usability
- `technical`: Architecture, infrastructure
- `business`: Business rules, processes
- `user`: User stories
- `constraint`: Limitations and restrictions

### Requirement Priorities (MoSCoW)

- `must`: Must have - critical requirements
- `should`: Should have - important but not critical
- `could`: Could have - desirable but not necessary
- `wont`: Won't have - explicitly excluded

### Requirement Statuses

- `new`: Newly identified
- `analyzed`: Reviewed and analyzed
- `approved`: Approved for implementation
- `in_progress`: Currently being implemented
- `completed`: Implementation complete
- `deferred`: Postponed

### Usage

```go
// Store a requirement
requirementsActivities.StoreRequirement(ctx, StoreRequirementRequest{
    ProjectID: "my-project",
    Requirement: Requirement{
        ID:          "REQ-001",
        Title:       "User Authentication",
        Description: "Users must be able to log in with email and password",
        Type:        RequirementTypeFunctional,
        Priority:    RequirementPriorityMust,
        Status:      RequirementStatusApproved,
        Category:    "Security",
        Tags:        []string{"auth", "security"},
        AcceptanceCriteria: []string{
            "User can register with email",
            "User can log in with credentials",
            "Failed attempts are logged",
        },
    },
})

// Update requirement status
requirementsActivities.UpdateRequirementStatus(ctx, UpdateRequirementStatusRequest{
    ProjectID:     "my-project",
    RequirementID: "REQ-001",
    Status:        RequirementStatusInProgress,
    Notes:         "Starting implementation",
})

// Link implementation
requirementsActivities.LinkImplementation(ctx, LinkImplementationRequest{
    ProjectID:     "my-project",
    RequirementID: "REQ-001",
    FilePath:      "/src/auth/login.go",
    FunctionName:  "HandleLogin",
    LineNumbers:   []int{45, 120},
    CommitHash:    "abc123",
})

// Get summary
summary, _ := requirementsActivities.GetRequirementsSummary(ctx, "my-project")
// Returns: counts by status, category breakdown, completion percentage
```

## Framework Learning

The framework learning feature allows Claude to analyze and learn any framework, generating structured knowledge that can be used to guide development.

### Learning Sources

- **Documentation URLs**: Official framework documentation
- **Code Repositories**: Example projects and source code
- **Local Code**: Existing project implementations

### Knowledge Extraction

- Framework patterns and idioms
- Best practices and anti-patterns
- Code examples for common tasks
- Configuration options
- Common pitfalls and solutions
- Integration guides

### Usage

```go
// Learn a framework from documentation
frameworkActivities.LearnFramework(ctx, LearnFrameworkRequest{
    Name:            "Echo",
    Language:        "go",
    DocumentationURL: "https://echo.labstack.com/docs",
    Categories:       []string{"routing", "middleware", "handlers"},
})

// Learn from existing codebase
frameworkActivities.LearnFramework(ctx, LearnFrameworkRequest{
    Name:         "Internal API Framework",
    Language:     "go",
    CodebasePath: "/path/to/project",
    Categories:   []string{"patterns", "conventions"},
})

// Get stored framework knowledge
knowledge, _ := frameworkActivities.GetFrameworkKnowledge(ctx, GetFrameworkKnowledgeRequest{
    Name: "Echo",
})

// Generate a prompt for Claude with framework knowledge
prompt, _ := frameworkActivities.GenerateFrameworkPrompt(ctx, GenerateFrameworkPromptRequest{
    FrameworkName: "Echo",
    TaskType:      "create_endpoint",
    Context:       "Need to create a REST endpoint for user management",
})

// List all learned frameworks
frameworks, _ := frameworkActivities.ListFrameworks(ctx)
```

### Framework Knowledge Structure

```json
{
  "name": "Echo",
  "version": "v4.11",
  "language": "go",
  "description": "High performance, minimalist Go web framework",
  "patterns": [
    {
      "name": "Route Grouping",
      "description": "Group related routes with common prefix and middleware",
      "code_example": "g := e.Group(\"/api\", middleware.Logger())",
      "when_to_use": "When organizing routes by feature or version",
      "category": "routing"
    }
  ],
  "best_practices": [
    "Use context for request-scoped values",
    "Implement custom error handler for consistent responses"
  ],
  "code_examples": [
    {
      "title": "Basic REST Endpoint",
      "code": "func getUser(c echo.Context) error { ... }",
      "explanation": "Handler function pattern for Echo"
    }
  ],
  "common_pitfalls": [
    {
      "issue": "Forgetting to return errors",
      "solution": "Always return c.JSON() or error from handlers"
    }
  ]
}
```

## Cloud Storage

The cloud storage feature provides a unified interface for storing and serving files across different backends.

### Supported Backends

- **Local**: Local filesystem storage with file:// URLs
- **S3**: Amazon S3 and compatible services
- **MinIO**: Self-hosted S3-compatible storage
- **GCS**: Google Cloud Storage (planned)
- **Azure**: Azure Blob Storage (planned)

### Usage

```go
// Upload a file
storageActivities.Upload(ctx, UploadFileRequest{
    Content:     []byte("file content"),
    Bucket:      "my-bucket",
    Path:        "documents/report.pdf",
    ContentType: "application/pdf",
    Metadata:    map[string]string{"author": "claude"},
    Public:      false,
})

// Generate upload URL (for direct client uploads)
storageActivities.GenerateUploadURL(ctx, GenerateUploadURLRequest{
    Bucket:           "my-bucket",
    Path:             "uploads/image.png",
    ContentType:      "image/png",
    ExpirationSeconds: 3600, // 1 hour
})

// Generate download URL (signed URL for temporary access)
storageActivities.GenerateDownloadURL(ctx, GenerateDownloadURLRequest{
    Bucket:            "my-bucket",
    Path:              "documents/report.pdf",
    ExpirationSeconds: 3600,
})

// List files
storageActivities.ListFiles(ctx, ListStorageFilesRequest{
    Bucket:    "my-bucket",
    Prefix:    "documents/",
    Recursive: true,
})
```

### Configuration

```yaml
storage:
  enabled: true
  type: "local"  # local, s3, minio
  local:
    base_path: "./storage"
    max_file_size: 104857600  # 100MB
  s3:
    region: "us-east-1"
    endpoint: ""  # For MinIO: "http://localhost:9000"
    access_key_id: ""
    secret_access_key: ""
    bucket: "my-bucket"
    use_path_style: false  # true for MinIO
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `FEATURE_STORAGE` | Enable/disable cloud storage |
| `STORAGE_TYPE` | Storage backend (local, s3, minio) |
| `STORAGE_LOCAL_PATH` | Base path for local storage |
| `S3_ENDPOINT` | Custom S3 endpoint for MinIO |
| `AWS_REGION` | AWS region |
| `AWS_ACCESS_KEY_ID` | AWS access key |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key |
| `S3_BUCKET` | Default S3 bucket |

## RAG (Retrieval-Augmented Generation)

The RAG feature enables semantic search over documents using vector embeddings. This is essential for building AI applications that need to retrieve relevant context from large document collections.

### Features

- **Document Chunking**: Split documents into semantic chunks (sentence, paragraph, code-aware)
- **Embedding Generation**: Generate embeddings via OpenAI, Cohere, or local models
- **Vector Search**: Find semantically similar content
- **Importance Weighting**: Weight chunks by importance (0-1 scale)
- **Context Retrieval**: Get surrounding chunks for better context
- **Namespaces**: Isolate search by user, project, or category

### Usage

```go
// Index a document
ragActivities.IndexDocument(ctx, IndexDocumentRequest{
    Content:   "Your document content...",
    Title:     "Technical Specification",
    Namespace: "project-123",
    Keywords:  []string{"api", "authentication"},
    ChunkSize: 1000,
})

// Search for relevant content
ragActivities.Search(ctx, SearchRequest{
    Query:              "How does authentication work?",
    Namespace:          "project-123",
    TopK:               5,
    MinScore:           0.7,
    IncludeContext:     true,
    ContextBefore:      1,
    ContextAfter:       1,
    WeightByImportance: true,
})

// Generate RAG prompt for Claude
ragActivities.GenerateRAGPrompt(ctx, GenerateRAGPromptRequest{
    Query:            "Explain the authentication flow",
    SearchResults:    searchResults,
    MaxContextTokens: 4000,
    IncludeSources:   true,
})

// Delete indexed document
ragActivities.DeleteDocument(ctx, DeleteDocumentRequest{
    DocumentID: "doc-123",
    Namespace:  "project-123",
})
```

### Chunking Strategies

| Strategy | Description | Best For |
|----------|-------------|----------|
| `fixed` | Fixed character size | General text |
| `sentence` | Sentence boundaries | Prose, documentation |
| `paragraph` | Paragraph boundaries | Articles, reports |
| `semantic` | Semantic sections | Mixed content |
| `code` | Function/class boundaries | Source code |
| `markdown` | Header-based sections | Markdown docs |

### Embedding Models

| Provider | Model | Dimensions |
|----------|-------|------------|
| OpenAI | text-embedding-3-small | 1536 |
| OpenAI | text-embedding-3-large | 3072 |
| OpenAI | text-embedding-ada-002 | 1536 |
| Cohere | embed-english-v3.0 | 1024 |
| Cohere | embed-multilingual-v3.0 | 1024 |

### Configuration

```yaml
rag:
  enabled: true
  default_namespace: "default"
  embedding:
    provider: "openai"
    model: "text-embedding-3-small"
    api_key: ""  # Or use EMBEDDING_API_KEY env var
    dimensions: 1536
    batch_size: 100
  chunking:
    strategy: "sentence"
    chunk_size: 1000
    chunk_overlap: 200
    min_chunk_size: 100
    max_chunk_size: 2000
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `FEATURE_RAG` | Enable/disable RAG |
| `EMBEDDING_PROVIDER` | Embedding provider (openai, cohere, local) |
| `EMBEDDING_MODEL` | Model name |
| `EMBEDDING_API_KEY` | API key for embedding service |
| `EMBEDDING_ENDPOINT` | Custom endpoint for local models |

## CLI Commands

### Run a Workflow

```bash
claude-orchestrator run [flags]

Flags:
  -t, --title string        Task title (required)
  -d, --description string  Task description
  -a, --max-agents int      Maximum number of agents (default 10)
      --timeout int         Workflow timeout in minutes (default 60)
      --parallel            Execute independent tasks in parallel (default true)
      --review              Enable code review agent (default true)
  -w, --wait                Wait for workflow completion
```

### Check Status

```bash
claude-orchestrator status <workflow-id> [flags]

Flags:
  -d, --detailed  Show detailed output
```

### List Workflows

```bash
claude-orchestrator list [flags]

Flags:
  -l, --limit int     Maximum workflows to list (default 20)
  -s, --status string Filter by status (Running, Completed, Failed, Cancelled)
```

### Cancel Workflow

```bash
claude-orchestrator cancel <workflow-id>
```

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/workflows` | Create new workflow |
| GET | `/api/v1/workflows` | List workflows |
| GET | `/api/v1/workflows/{id}` | Get workflow details |
| DELETE | `/api/v1/workflows/{id}` | Cancel workflow |

### Create Workflow Request

```json
{
  "title": "Task title",
  "description": "Detailed description",
  "max_agents": 10,
  "timeout_minutes": 60,
  "parallel": true,
  "review": true,
  "context": {
    "key": "value"
  }
}
```

## Agent Types

### Orchestrator (Queen)
Coordinates the entire workflow. Decomposes tasks, assigns to agents, and aggregates results.

### Planner
Creates detailed implementation plans and architecture designs.

### Researcher
Gathers information, analyzes codebases, and provides context.

### Coder
Generates production-ready code based on specifications.

### Reviewer
Reviews code for bugs, security issues, and best practices.

### Executor
Runs commands, tests, and manages git operations.

## Memory System

The orchestrator includes a flexible memory system for sharing context between agents:

```go
// Store shared memory
memoryActivities.Store(ctx, StoreMemoryRequest{
    Key:       "design",
    Value:     "API design document...",
    Namespace: "workflow",
    Tags:      []string{"architecture", "api"},
    TTLSeconds: 3600,
})

// Query memory
memoryActivities.Query(ctx, QueryMemoryRequest{
    Pattern:   "design:*",
    Namespace: "workflow",
    Tags:      []string{"architecture"},
})

// Vector search for semantic similarity
memoryActivities.VectorSearch(ctx, VectorSearchRequest{
    Query:     "authentication implementation",
    Namespace: "code",
    TopK:      5,
})
```

### Memory Namespaces

- `global`: Shared across all workflows
- `workflow:{id}`: Scoped to a specific workflow
- `workflow:{id}:agent:{agentId}`: Scoped to a specific agent
- `workflow:{id}:shared`: Shared between agents in a workflow

### Memory Backends

- **In-Memory**: Default, suitable for development and single-instance deployments
- **PostgreSQL**: Persistent storage with vector search support (pgvector)

## Project Structure

```
claude-orchestrator/
├── cmd/
│   ├── worker/main.go       # Temporal worker process
│   ├── cli/main.go          # CLI tool
│   └── api/main.go          # REST API server
├── internal/
│   ├── workflow/
│   │   ├── orchestrator.go  # Queen/Parent workflow
│   │   ├── agent.go         # Generic agent workflow
│   │   ├── planner.go       # Planner agent
│   │   ├── researcher.go    # Researcher agent
│   │   ├── coder.go         # Coder agent
│   │   ├── reviewer.go      # Reviewer agent
│   │   └── executor.go      # Executor agent
│   ├── activity/
│   │   ├── claude.go        # Claude API activities
│   │   ├── filesystem.go    # File operations
│   │   ├── git.go           # Git operations
│   │   ├── memory.go        # Memory operations
│   │   ├── document.go      # Document analysis
│   │   ├── requirements.go  # Requirements tracking
│   │   ├── framework.go     # Framework learning
│   │   ├── storage.go       # Cloud storage activities
│   │   └── rag.go           # RAG activities
│   ├── config/
│   │   ├── config.go        # Configuration management
│   │   └── config_test.go   # Configuration tests
│   ├── memory/
│   │   ├── store.go         # Memory interface
│   │   ├── memory.go        # In-memory implementation
│   │   ├── memory_test.go   # Memory tests
│   │   └── postgres.go      # PostgreSQL implementation
│   ├── storage/
│   │   ├── store.go         # Storage interface
│   │   ├── local.go         # Local filesystem storage
│   │   └── s3.go            # S3/MinIO storage
│   ├── rag/
│   │   ├── chunker.go       # Document chunking
│   │   └── embeddings.go    # Embedding generation
│   └── domain/
│       ├── agent.go         # Agent types
│       ├── task.go          # Task definitions
│       └── message.go       # Messages
├── pkg/
│   └── claude/
│       ├── client.go        # Claude API client
│       └── client_test.go   # Client tests
├── Dockerfile               # Multi-stage Docker build
├── docker-compose.yml       # Full stack deployment
├── Makefile                 # Build automation
└── init-db.sql              # PostgreSQL initialization
```

## Docker Deployment

### Using Docker Compose

```bash
# Set your API key
export ANTHROPIC_API_KEY=your-api-key

# Start all services (Temporal, PostgreSQL, MinIO, Worker, API)
docker-compose up -d

# View logs
docker-compose logs -f worker

# Start with full stack (includes Redis)
docker-compose --profile full up -d

# Start with development tools
docker-compose --profile dev up -d

# Use CLI
docker-compose --profile cli run cli status <workflow-id>
```

### Services

| Service | Port | Description |
|---------|------|-------------|
| temporal | 7233 | Temporal server gRPC |
| temporal-ui | 8233 | Temporal Web UI |
| postgres | 5432 | PostgreSQL database |
| minio | 9000/9001 | MinIO API/Console |
| worker | - | Orchestrator worker |
| api | 8080 | REST API server |
| redis | 6379 | Redis cache (full profile) |

### Building Images

```bash
# Build all images
make docker-build

# Or build individually
docker build --target worker -t claude-orchestrator-worker .
docker build --target api -t claude-orchestrator-api .
docker build --target cli -t claude-orchestrator-cli .
docker build --target all -t claude-orchestrator .
```

## Kubernetes Deployment (Helm)

### Prerequisites

- Kubernetes cluster 1.19+
- Helm 3.x
- kubectl configured

### Installing the Chart

```bash
# Add required repositories
helm repo add bitnami https://charts.bitnami.com/bitnami

# Install with default values
helm install claude-orchestrator ./helm/claude-orchestrator \
  --set secrets.anthropicApiKey=your-api-key

# Install with custom values
helm install claude-orchestrator ./helm/claude-orchestrator \
  -f my-values.yaml

# Install with external Temporal
helm install claude-orchestrator ./helm/claude-orchestrator \
  --set temporal.enabled=false \
  --set temporal.externalAddress=temporal.example.com:7233 \
  --set secrets.anthropicApiKey=your-api-key
```

### Configuration

Create a `values.yaml` file to customize:

```yaml
# Worker configuration
worker:
  replicaCount: 2
  resources:
    requests:
      memory: "256Mi"
      cpu: "100m"
    limits:
      memory: "512Mi"
      cpu: "500m"

# API configuration
api:
  replicaCount: 2
  ingress:
    enabled: true
    className: nginx
    hosts:
      - host: orchestrator.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - hosts:
          - orchestrator.example.com
        secretName: orchestrator-tls

# Feature toggles
config:
  features:
    storage: true
    rag: true
    documentAnalysis: true
    requirementsTracking: true
    frameworkLearning: true
  storage:
    type: minio  # local, s3, minio
  rag:
    embeddingProvider: openai
    embeddingModel: text-embedding-3-small

# Dependencies
postgresql:
  enabled: true
  auth:
    postgresPassword: "changeme"

minio:
  enabled: true
  auth:
    rootUser: minioadmin
    rootPassword: minioadmin

# Secrets (use sealed-secrets or external-secrets in production)
secrets:
  anthropicApiKey: ""
  embeddingApiKey: ""
```

### Upgrading

```bash
helm upgrade claude-orchestrator ./helm/claude-orchestrator -f my-values.yaml
```

### Uninstalling

```bash
helm uninstall claude-orchestrator
```

## MCP Integration (Model Context Protocol)

The orchestrator integrates with Claude Code via MCP servers for enhanced tool access.

### Configuration

The MCP configuration is in `.claude/mcp.json`:

```json
{
  "mcpServers": {
    "temporal": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-temporal"],
      "env": {
        "TEMPORAL_ADDRESS": "localhost:7233",
        "TEMPORAL_NAMESPACE": "default"
      }
    },
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-fs"],
      "env": {
        "MCP_FS_ROOT": "."
      }
    },
    "git": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-git"],
      "env": {
        "GIT_REPO_PATH": "."
      }
    },
    "memory": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-memory"]
    }
  }
}
```

### Available MCP Servers

| Server | Description | Usage |
|--------|-------------|-------|
| `temporal` | Temporal workflow operations | Query, signal, manage workflows |
| `temporal-cloud` | Temporal Cloud management | Namespaces, users, cloud resources |
| `filesystem` | File system operations | Read, write, manage project files |
| `git` | Git operations | Commits, branches, diffs, history |
| `memory` | Persistent memory | Store context across sessions |
| `postgres` | PostgreSQL operations | Query and manage database |

### Using with Claude Code

1. Ensure the MCP configuration is in your project's `.claude/mcp.json`
2. Start the required services (Temporal, PostgreSQL, etc.)
3. Claude Code will automatically connect to configured MCP servers
4. Use Claude to interact with workflows:

```
"Start a new orchestration workflow for building a REST API"
"Check the status of workflow xyz-123"
"List all running workflows"
"Query the agent memory for design decisions"
```

### Temporal MCP Features

The Temporal MCP server provides:

- **Workflow Management**: Start, query, signal, cancel workflows
- **Activity Monitoring**: View activity status and history
- **Namespace Operations**: Manage namespaces and configuration
- **Search**: Find workflows by status, type, or custom attributes

For more information, see the [Temporal MCP documentation](https://temporal.mcp.kapa.ai).

## Observability

### Logging

The orchestrator uses structured logging via [zap](https://github.com/uber-go/zap) with configurable levels and formats.

#### Configuration

```yaml
logging:
  level: "info"         # debug, info, warn, error
  format: "json"        # json, console, text
  output: "stdout"      # stdout, stderr, or file path
  add_caller: true      # Include caller information
  add_stacktrace: false # Include stack trace for errors
  development: false    # Development mode (more verbose)
  sampling:
    enabled: false      # Enable log sampling for high-volume
    initial: 100        # Log first N entries per second
    thereafter: 100     # Then log every Mth entry
```

#### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |
| `LOG_FORMAT` | Output format (json, console, text) | `json` |
| `LOG_OUTPUT` | Output destination | `stdout` |
| `LOG_DEVELOPMENT` | Enable development mode | `false` |

#### Log Fields

Logs include contextual fields:

```json
{
  "time": "2024-01-15T10:30:00Z",
  "level": "info",
  "caller": "worker/main.go:123",
  "msg": "Workflow started",
  "workflow_id": "orchestrator-abc123",
  "run_id": "xyz789",
  "agent_type": "planner"
}
```

### Metrics

The orchestrator exposes Prometheus metrics for monitoring workflows, activities, and system health.

#### Configuration

```yaml
metrics:
  enabled: true
  address: ":9090"
  path: "/metrics"
  namespace: "claude_orchestrator"
  enable_go_metrics: true
  enable_process_metrics: true
```

#### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `METRICS_ENABLED` | Enable metrics endpoint | `true` |
| `METRICS_ADDRESS` | Metrics server address | `:9090` |
| `METRICS_PATH` | Metrics endpoint path | `/metrics` |

#### Available Metrics

**Workflow Metrics:**
- `claude_orchestrator_workflows_started_total` - Total workflows started
- `claude_orchestrator_workflows_completed_total` - Total workflows completed
- `claude_orchestrator_workflows_failed_total` - Total workflows failed
- `claude_orchestrator_workflow_duration_seconds` - Workflow duration histogram
- `claude_orchestrator_workflows_active` - Currently active workflows

**Activity Metrics:**
- `claude_orchestrator_activities_started_total` - Total activities started
- `claude_orchestrator_activities_completed_total` - Total activities completed
- `claude_orchestrator_activities_failed_total` - Total activities failed
- `claude_orchestrator_activity_duration_seconds` - Activity duration histogram

**Agent Metrics:**
- `claude_orchestrator_agent_tasks_assigned_total` - Tasks assigned to agents
- `claude_orchestrator_agent_tasks_completed_total` - Tasks completed by agents
- `claude_orchestrator_agent_task_duration_seconds` - Agent task duration

**Claude API Metrics:**
- `claude_orchestrator_claude_api_requests_total` - Total API requests
- `claude_orchestrator_claude_api_errors_total` - Total API errors
- `claude_orchestrator_claude_api_latency_seconds` - API latency histogram
- `claude_orchestrator_claude_api_tokens_input_total` - Input tokens used
- `claude_orchestrator_claude_api_tokens_output_total` - Output tokens used

**HTTP API Metrics:**
- `claude_orchestrator_http_requests_total` - Total HTTP requests
- `claude_orchestrator_http_request_duration_seconds` - Request duration

#### Prometheus Integration

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'worker'
    static_configs:
      - targets: ['worker:9090']
  - job_name: 'api'
    static_configs:
      - targets: ['api:9090']
```

#### Grafana Dashboard

Start the monitoring stack:

```bash
docker-compose --profile monitoring up -d
```

Access:
- Prometheus: http://localhost:9091
- Grafana: http://localhost:3000 (admin/admin)

## Development

### Running Tests

```bash
# Run all tests
make test

# Run with verbose output
go test -v ./...

# Run specific package tests
go test ./internal/memory/...
go test ./internal/config/...
go test ./pkg/claude/...
```

### Building

```bash
# Build all binaries
make build

# Build for specific OS/Arch
GOOS=linux GOARCH=amd64 go build -o bin/worker-linux ./cmd/worker
```

### Linting

```bash
make lint
```

### Debugging with Temporal UI

The Temporal UI is available at `http://localhost:8233` when running the local development server. It provides:

- Workflow execution history
- Activity execution details
- Signal/Query handling
- Workflow state inspection

## Extending

### Adding a New Agent Type

1. Add the agent type to `internal/domain/agent.go`:
```go
const AgentTypeCustom AgentType = "custom"
```

2. Create a new workflow in `internal/workflow/custom.go`:
```go
func CustomWorkflow(ctx workflow.Context, input AgentWorkflowInput) (*AgentWorkflowOutput, error) {
    // Implementation
}
```

3. Register the workflow in `cmd/worker/main.go`:
```go
w.RegisterWorkflow(workflow.CustomWorkflow)
```

4. Add the agent to the orchestrator's routing logic in `orchestrator.go`.

### Adding New Activities

1. Create activities in `internal/activity/`:
```go
func (a *CustomActivities) DoSomething(ctx context.Context, req Request) (*Result, error) {
    // Implementation
}
```

2. Register in the worker:
```go
w.RegisterActivity(customActivities.DoSomething)
```

### Adding New Features

1. Add feature configuration to `internal/config/config.go`:
```go
type MyFeature struct {
    Enabled bool   `json:"enabled"`
    Setting string `json:"setting"`
}
```

2. Add environment variable support in `LoadConfig`:
```go
if envValue := os.Getenv("FEATURE_MY_FEATURE"); envValue != "" {
    cfg.Features.MyFeature.Enabled = parseBool(envValue)
}
```

3. Check feature status in worker:
```go
if cfg.IsFeatureEnabled("my_feature") {
    // Initialize feature
}
```

## Best Practices

1. **Task Descriptions**: Provide detailed, specific task descriptions for better decomposition
2. **Memory Usage**: Use appropriate namespaces and TTLs for memory entries
3. **Error Handling**: The system handles retries automatically; design activities to be idempotent
4. **Monitoring**: Use Temporal UI to monitor workflow execution and debug issues
5. **Resource Limits**: Configure `MaxAgents` based on your API rate limits
6. **Feature Flags**: Use configuration to enable/disable features per environment
7. **Requirements Tracking**: Link implementations to requirements for traceability
8. **Framework Learning**: Pre-load framework knowledge for commonly used frameworks

## Troubleshooting

### Worker Not Processing Tasks

1. Check that the worker is connected to Temporal
2. Verify the task queue name matches
3. Check for errors in worker logs

### Claude API Errors

1. Verify your `ANTHROPIC_API_KEY` is set correctly
2. Check rate limits (the system includes automatic retries)
3. Review the activity heartbeat logs

### Memory Issues

1. Use appropriate TTLs for temporary data
2. Clear namespace memory after workflow completion
3. Monitor memory store size

### Feature Not Working

1. Check if the feature is enabled in configuration
2. Verify environment variables are set correctly
3. Check worker logs for registration messages

### Database Connection Issues

1. Verify PostgreSQL is running and accessible
2. Check connection string parameters
3. Ensure database and user exist
4. Check SSL mode settings

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Submit a pull request

## License

MIT License - see LICENSE file for details.
