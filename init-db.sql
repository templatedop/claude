-- Initialize the database for Claude Orchestrator

-- Create memory database
CREATE DATABASE claude_orchestrator;

-- Connect to the memory database
\c claude_orchestrator;

-- Enable pgvector extension for embeddings
CREATE EXTENSION IF NOT EXISTS vector;

-- Create memory schema
CREATE SCHEMA IF NOT EXISTS memory;

-- Create memories table
CREATE TABLE IF NOT EXISTS memory.memories (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'working',
    namespace TEXT NOT NULL DEFAULT 'global',
    tags TEXT[] DEFAULT '{}',
    embedding vector(1536),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    access_count INTEGER NOT NULL DEFAULT 0
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_memories_namespace ON memory.memories(namespace);
CREATE INDEX IF NOT EXISTS idx_memories_type ON memory.memories(type);
CREATE INDEX IF NOT EXISTS idx_memories_tags ON memory.memories USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memory.memories(created_at);
CREATE INDEX IF NOT EXISTS idx_memories_expires_at ON memory.memories(expires_at) WHERE expires_at IS NOT NULL;

-- Create function to update updated_at timestamp
CREATE OR REPLACE FUNCTION memory.update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create trigger for updated_at
DROP TRIGGER IF EXISTS update_memories_updated_at ON memory.memories;
CREATE TRIGGER update_memories_updated_at
    BEFORE UPDATE ON memory.memories
    FOR EACH ROW
    EXECUTE FUNCTION memory.update_updated_at_column();

-- Grant permissions
GRANT ALL ON SCHEMA memory TO temporal;
GRANT ALL ON ALL TABLES IN SCHEMA memory TO temporal;
