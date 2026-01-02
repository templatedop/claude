# Claude Code Integration Guide

This guide explains how to use Claude Code with the Claude Orchestrator, allowing you to leverage your Claude subscription instead of API credits.

## Overview

The Claude Orchestrator supports two providers for interacting with Claude:

| Provider | Billing | Authentication | Best For |
|----------|---------|----------------|----------|
| `api` | Pay-per-token API credits | `ANTHROPIC_API_KEY` | Production, Docker, Kubernetes |
| `claude_code` | Claude subscription | `claude login` | Development, cost savings |

## How Claude Code Integration Works

When using `claude_code` provider:

1. The orchestrator spawns the `claude` CLI as a subprocess
2. Removes `ANTHROPIC_API_KEY` from the environment to force subscription auth
3. Uses `--print` and `--output-format json` for non-interactive operation
4. Parses JSON responses from the CLI

```
┌─────────────────────────────────────────────────────────────┐
│                    Claude Orchestrator                       │
│  ┌─────────────────┐    ┌─────────────────────────────────┐ │
│  │ Temporal Worker │───▶│ ClaudeCodeClient                │ │
│  └─────────────────┘    │  • Spawns `claude` CLI          │ │
│                         │  • Uses subscription auth        │ │
│                         │  • Parses JSON output           │ │
│                         └──────────────┬──────────────────┘ │
└────────────────────────────────────────┼────────────────────┘
                                         │
                                         ▼
                              ┌──────────────────────┐
                              │   Claude Code CLI    │
                              │  (uses ~/.claude/)   │
                              └──────────────────────┘
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CLAUDE_PROVIDER` | Provider type: `api` or `claude_code` | `api` |
| `ANTHROPIC_API_KEY` | API key (only for `api` provider) | - |
| `CLAUDE_MODEL` | Claude model to use | `claude-sonnet-4-20250514` |
| `WORKING_DIR` | Working directory for Claude Code | `.` |
| `CLAUDE_ALLOWED_TOOLS` | Comma-separated list of allowed tools | `Read,Write,Bash,Glob,Grep` |

### Configuration File

```yaml
claude:
  provider: "claude_code"  # or "api"
  model: "claude-sonnet-4-20250514"
  max_tokens: 4096
  temperature: 0.7
  working_dir: "/app/workspace"
  allowed_tools:
    - "Read"
    - "Write"
    - "Bash"
    - "Glob"
    - "Grep"
```

## Usage Scenarios

### Scenario 1: Local Development (Recommended)

For development, run the worker natively without Docker:

```bash
# 1. Install Claude Code CLI
npm install -g @anthropic-ai/claude-code

# 2. Authenticate with your Claude subscription
claude login

# 3. Build and run the worker
go build -o bin/worker ./cmd/worker

# 4. Start with Claude Code provider
export CLAUDE_PROVIDER=claude_code
export TEMPORAL_ADDRESS=localhost:7233
export WORKING_DIR=$(pwd)

./bin/worker
```

### Scenario 2: Docker with API Provider (Production)

For production, use the API provider in Docker:

```bash
# Set provider to API
export CLAUDE_PROVIDER=api
export ANTHROPIC_API_KEY=your-api-key

# Start services
docker-compose up -d
```

### Scenario 3: Docker with Claude Code (Advanced)

For Docker with Claude Code, you need to:

1. Install Claude Code in the Docker image
2. Mount your authentication credentials

```bash
# 1. First, authenticate on your host machine
npm install -g @anthropic-ai/claude-code
claude login

# 2. Use the Claude Code-enabled docker-compose
docker-compose -f docker-compose.claude-code.yml up -d
```

**Requirements:**
- Claude Code CLI authenticated on your host machine
- `~/.claude/` directory with valid auth tokens
- Volume mount for credentials

### Scenario 4: Hybrid Approach

Run infrastructure in Docker, worker natively:

```bash
# 1. Start infrastructure only
docker-compose up -d temporal postgres minio

# 2. Run worker locally with Claude Code
export CLAUDE_PROVIDER=claude_code
export TEMPORAL_ADDRESS=localhost:7233
export WORKING_DIR=$(pwd)

./bin/worker
```

## Docker Images

### Standard Images (API Provider)

```dockerfile
# Worker
docker build --target worker -t claude-orchestrator-worker .

# API
docker build --target api -t claude-orchestrator-api .
```

### Claude Code Image

Use the `Dockerfile.claudecode` for Claude Code support:

```bash
# Build with Claude Code CLI installed
docker build -f Dockerfile.claudecode -t claude-orchestrator-claudecode .
```

## Kubernetes Considerations

For Kubernetes deployments with Claude Code:

### Option 1: API Provider (Recommended)

```yaml
# values.yaml
config:
  claude:
    provider: "api"

secrets:
  anthropicApiKey: "your-api-key"
```

### Option 2: Claude Code with Secrets

1. Create a secret with your Claude auth:

```bash
# On your authenticated host
kubectl create secret generic claude-auth \
  --from-file=config.json=$HOME/.claude/config.json \
  --from-file=credentials.json=$HOME/.claude/credentials.json
```

2. Mount in the worker pod:

```yaml
# values.yaml
config:
  claude:
    provider: "claude_code"

worker:
  extraVolumes:
    - name: claude-auth
      secret:
        secretName: claude-auth
  extraVolumeMounts:
    - name: claude-auth
      mountPath: /home/orchestrator/.claude
      readOnly: true
```

**Warning:** This approach exposes your subscription credentials. Only use in trusted environments.

## Troubleshooting

### "claude CLI not found"

Ensure Claude Code CLI is installed:

```bash
npm install -g @anthropic-ai/claude-code
```

### "authentication failed"

Re-authenticate with Claude:

```bash
claude login
```

### Docker: "cannot find ~/.claude/"

Mount the auth directory:

```bash
docker run -v $HOME/.claude:/home/orchestrator/.claude:ro ...
```

### Permission Denied on Auth Files

Ensure proper permissions:

```bash
chmod 600 ~/.claude/*
```

### "insufficient balance" Error

This error occurs when using `api` provider. Switch to `claude_code`:

```bash
export CLAUDE_PROVIDER=claude_code
```

Or add credits to your API account at console.anthropic.com.

## Provider Auto-Detection

If `CLAUDE_PROVIDER` is not set, the orchestrator auto-detects:

1. If `ANTHROPIC_API_KEY` is set → uses `api` provider
2. If no API key → uses `claude_code` provider

```bash
# Auto-uses API (key is present)
export ANTHROPIC_API_KEY=sk-...
./bin/worker

# Auto-uses Claude Code (no key)
unset ANTHROPIC_API_KEY
./bin/worker
```

## Feature Compatibility

| Feature | API Provider | Claude Code Provider |
|---------|--------------|---------------------|
| Core completions | ✅ | ✅ |
| Task decomposition | ✅ | ✅ |
| Code generation | ✅ | ✅ |
| Code analysis | ✅ | ✅ |
| Document analysis | ✅ | ⚠️ (coming soon) |
| Framework learning | ✅ | ⚠️ (coming soon) |
| Token tracking | ✅ | ❌ (not available) |

Note: Claude Code provider does not report detailed token usage since it uses the subscription model.

## API vs Claude Code: Comparison

| Aspect | API Provider | Claude Code Provider |
|--------|--------------|---------------------|
| **Cost Model** | Pay per token | Subscription included |
| **Setup** | API key only | Install CLI + login |
| **Docker Support** | Full | Requires auth mount |
| **Kubernetes** | Easy | Complex |
| **Offline Auth** | Works anywhere | Requires login |
| **Token Metrics** | Full tracking | Not available |
| **Rate Limits** | API limits | Subscription limits |

## Best Practices

1. **Development**: Use `claude_code` provider to save on API costs
2. **Production**: Use `api` provider for reliability and ease of deployment
3. **CI/CD**: Always use `api` provider (no interactive login possible)
4. **Local Testing**: Use `claude_code` with local worker + Docker infrastructure
5. **Cost Optimization**: Use `claude_code` for development, `api` for production

## Security Considerations

### API Provider
- Store `ANTHROPIC_API_KEY` in secrets management (Vault, AWS Secrets Manager, etc.)
- Use environment-specific keys
- Rotate keys regularly

### Claude Code Provider
- Never commit `~/.claude/` contents to version control
- Use read-only mounts in Docker
- Limit container network access
- Consider using short-lived auth tokens

## Example: Complete Development Setup

```bash
# 1. Clone and setup
git clone https://github.com/anthropics/claude-orchestrator
cd claude-orchestrator

# 2. Install Claude Code CLI
npm install -g @anthropic-ai/claude-code

# 3. Login to Claude
claude login

# 4. Start infrastructure
docker-compose up -d temporal postgres minio

# 5. Build the worker
go build -o bin/worker ./cmd/worker

# 6. Run with Claude Code
export CLAUDE_PROVIDER=claude_code
export TEMPORAL_ADDRESS=localhost:7233
export WORKING_DIR=$(pwd)/workspace

./bin/worker
```

## Migrating Between Providers

### From API to Claude Code

```bash
# Before
export CLAUDE_PROVIDER=api
export ANTHROPIC_API_KEY=sk-...

# After
export CLAUDE_PROVIDER=claude_code
unset ANTHROPIC_API_KEY
claude login  # Ensure authenticated
```

### From Claude Code to API

```bash
# Before
export CLAUDE_PROVIDER=claude_code

# After
export CLAUDE_PROVIDER=api
export ANTHROPIC_API_KEY=sk-...  # Get from console.anthropic.com
```

No workflow changes are needed - both providers use the same interface.
