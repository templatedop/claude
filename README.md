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

- **Task Decomposition**: Automatically breaks down complex tasks into subtasks
- **Specialized Agents**: Different agent types for planning, research, coding, reviewing, and execution
- **Parallel Execution**: Execute independent tasks concurrently
- **Memory System**: Shared memory between agents with namespacing
- **Fault Tolerance**: Built-in retries and durability via Temporal
- **Multiple Interfaces**: CLI, REST API, and direct Temporal workflow execution

## Prerequisites

- Go 1.21 or later
- Temporal server (can run locally with Docker)
- Anthropic API key for Claude access

## Installation

```bash
# Clone the repository
git clone https://github.com/anthropics/claude-orchestrator.git
cd claude-orchestrator

# Install dependencies
go mod tidy

# Build the binaries
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

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ANTHROPIC_API_KEY` | Anthropic API key (required) | - |
| `TEMPORAL_ADDRESS` | Temporal server address | `localhost:7233` |
| `TEMPORAL_NAMESPACE` | Temporal namespace | `default` |
| `TASK_QUEUE` | Temporal task queue name | `claude-orchestrator` |
| `WORKING_DIR` | Working directory for file operations | `.` |
| `PORT` | API server port | `8080` |

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
```

### Memory Namespaces

- `global`: Shared across all workflows
- `workflow:{id}`: Scoped to a specific workflow
- `workflow:{id}:agent:{agentId}`: Scoped to a specific agent
- `workflow:{id}:shared`: Shared between agents in a workflow

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
│   │   └── memory.go        # Memory operations
│   ├── memory/
│   │   ├── store.go         # Memory interface
│   │   ├── memory.go        # In-memory implementation
│   │   └── postgres.go      # PostgreSQL implementation
│   └── domain/
│       ├── agent.go         # Agent types
│       ├── task.go          # Task definitions
│       └── message.go       # Messages
└── pkg/
    └── claude/
        └── client.go        # Claude API client
```

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build ./...
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

## Best Practices

1. **Task Descriptions**: Provide detailed, specific task descriptions for better decomposition
2. **Memory Usage**: Use appropriate namespaces and TTLs for memory entries
3. **Error Handling**: The system handles retries automatically; design activities to be idempotent
4. **Monitoring**: Use Temporal UI to monitor workflow execution and debug issues
5. **Resource Limits**: Configure `MaxAgents` based on your API rate limits

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

## License

MIT License - see LICENSE file for details.
