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

## Prerequisites

- Go 1.21 or later
- Temporal server (can run locally with Docker)
- Anthropic API key for Claude access
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

```bash
export ANTHROPIC_API_KEY=your-api-key
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

## Configuration

### Configuration File

Create a `config.yaml` file:

```yaml
temporal:
  address: "localhost:7233"
  namespace: "default"
  task_queue: "claude-orchestrator"

claude:
  model: "claude-sonnet-4-20250514"
  max_tokens: 4096

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
| `ANTHROPIC_API_KEY` | Anthropic API key (required) | - |
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
│   │   └── framework.go     # Framework learning
│   ├── config/
│   │   ├── config.go        # Configuration management
│   │   └── config_test.go   # Configuration tests
│   ├── memory/
│   │   ├── store.go         # Memory interface
│   │   ├── memory.go        # In-memory implementation
│   │   ├── memory_test.go   # Memory tests
│   │   └── postgres.go      # PostgreSQL implementation
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

# Start all services
docker-compose up -d

# View logs
docker-compose logs -f worker
```

### Building Images

```bash
# Build all images
make docker-build

# Or build individually
docker build --target worker -t claude-orchestrator-worker .
docker build --target api -t claude-orchestrator-api .
docker build --target cli -t claude-orchestrator-cli .
```

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
