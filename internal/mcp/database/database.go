// Package database implements an MCP server for database operations.
package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/mcp"
	_ "github.com/lib/pq"
)

// Config holds database configuration.
type Config struct {
	// Driver is the database driver (postgres, mysql, sqlite3)
	Driver string `json:"driver" yaml:"driver"`
	// DSN is the data source name / connection string
	DSN string `json:"dsn" yaml:"dsn"`
	// Host, Port, User, Password, Database for building DSN
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
	Database string `json:"database" yaml:"database"`
	SSLMode  string `json:"ssl_mode" yaml:"ssl_mode"`
	// MaxRows limits the number of rows returned in queries
	MaxRows int `json:"max_rows" yaml:"max_rows"`
	// ReadOnly restricts to SELECT queries only
	ReadOnly bool `json:"read_only" yaml:"read_only"`
	// AllowedTables restricts queries to specific tables
	AllowedTables []string `json:"allowed_tables" yaml:"allowed_tables"`
}

// Client is a database client.
type Client struct {
	db            *sql.DB
	driver        string
	maxRows       int
	readOnly      bool
	allowedTables map[string]bool
}

// NewClient creates a new database client.
func NewClient(cfg Config) (*Client, error) {
	driver := cfg.Driver
	if driver == "" {
		driver = "postgres"
	}

	dsn := cfg.DSN
	if dsn == "" {
		// Build DSN from components
		switch driver {
		case "postgres":
			sslMode := cfg.SSLMode
			if sslMode == "" {
				sslMode = "disable"
			}
			dsn = fmt.Sprintf(
				"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
				cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, sslMode,
			)
		case "mysql":
			dsn = fmt.Sprintf(
				"%s:%s@tcp(%s:%d)/%s",
				cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
			)
		default:
			return nil, fmt.Errorf("unsupported driver: %s", driver)
		}
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	maxRows := cfg.MaxRows
	if maxRows <= 0 {
		maxRows = 100
	}

	allowedTables := make(map[string]bool)
	for _, t := range cfg.AllowedTables {
		allowedTables[strings.ToLower(t)] = true
	}

	return &Client{
		db:            db,
		driver:        driver,
		maxRows:       maxRows,
		readOnly:      cfg.ReadOnly,
		allowedTables: allowedTables,
	}, nil
}

// Close closes the database connection.
func (c *Client) Close() error {
	return c.db.Close()
}

// QueryResult represents the result of a query.
type QueryResult struct {
	Columns  []string                 `json:"columns"`
	Rows     []map[string]interface{} `json:"rows"`
	RowCount int                      `json:"row_count"`
	Truncated bool                    `json:"truncated,omitempty"`
}

// ExecResult represents the result of an exec.
type ExecResult struct {
	RowsAffected int64 `json:"rows_affected"`
	LastInsertID int64 `json:"last_insert_id,omitempty"`
}

// TableInfo represents table metadata.
type TableInfo struct {
	Name    string       `json:"name"`
	Schema  string       `json:"schema,omitempty"`
	Columns []ColumnInfo `json:"columns"`
}

// ColumnInfo represents column metadata.
type ColumnInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key,omitempty"`
	Default    string `json:"default,omitempty"`
}

// Query executes a SELECT query.
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (*QueryResult, error) {
	// Validate read-only mode
	if c.readOnly {
		normalized := strings.ToUpper(strings.TrimSpace(query))
		if !strings.HasPrefix(normalized, "SELECT") &&
			!strings.HasPrefix(normalized, "WITH") &&
			!strings.HasPrefix(normalized, "EXPLAIN") {
			return nil, fmt.Errorf("only SELECT queries are allowed in read-only mode")
		}
	}

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	result := &QueryResult{
		Columns: columns,
		Rows:    make([]map[string]interface{}, 0),
	}

	// Prepare value holders
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	rowCount := 0
	for rows.Next() {
		if rowCount >= c.maxRows {
			result.Truncated = true
			break
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for readability
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			row[col] = val
		}

		result.Rows = append(result.Rows, row)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	result.RowCount = len(result.Rows)
	return result, nil
}

// Exec executes a non-SELECT query.
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) (*ExecResult, error) {
	if c.readOnly {
		return nil, fmt.Errorf("database is in read-only mode")
	}

	result, err := c.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("exec failed: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	lastInsertID, _ := result.LastInsertId()

	return &ExecResult{
		RowsAffected: rowsAffected,
		LastInsertID: lastInsertID,
	}, nil
}

// ListTables lists all tables in the database.
func (c *Client) ListTables(ctx context.Context) ([]string, error) {
	var query string
	switch c.driver {
	case "postgres":
		query = `
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_type = 'BASE TABLE'
			ORDER BY table_name`
	case "mysql":
		query = `SHOW TABLES`
	default:
		return nil, fmt.Errorf("unsupported driver for ListTables: %s", c.driver)
	}

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}

	return tables, nil
}

// DescribeTable returns metadata about a table.
func (c *Client) DescribeTable(ctx context.Context, tableName string) (*TableInfo, error) {
	// Check allowed tables if configured
	if len(c.allowedTables) > 0 {
		if !c.allowedTables[strings.ToLower(tableName)] {
			return nil, fmt.Errorf("access to table '%s' is not allowed", tableName)
		}
	}

	info := &TableInfo{
		Name:    tableName,
		Columns: make([]ColumnInfo, 0),
	}

	var query string
	switch c.driver {
	case "postgres":
		query = `
			SELECT
				c.column_name,
				c.data_type,
				c.is_nullable = 'YES' as nullable,
				COALESCE(c.column_default, '') as column_default,
				COALESCE(
					(SELECT true FROM information_schema.table_constraints tc
					 JOIN information_schema.key_column_usage kcu
					 ON tc.constraint_name = kcu.constraint_name
					 WHERE tc.table_name = c.table_name
					 AND kcu.column_name = c.column_name
					 AND tc.constraint_type = 'PRIMARY KEY'),
					false
				) as is_primary
			FROM information_schema.columns c
			WHERE c.table_name = $1
			AND c.table_schema = 'public'
			ORDER BY c.ordinal_position`
	default:
		return nil, fmt.Errorf("unsupported driver for DescribeTable: %s", c.driver)
	}

	rows, err := c.db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to describe table: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var col ColumnInfo
		if err := rows.Scan(&col.Name, &col.Type, &col.Nullable, &col.Default, &col.PrimaryKey); err != nil {
			return nil, err
		}
		info.Columns = append(info.Columns, col)
	}

	if len(info.Columns) == 0 {
		return nil, fmt.Errorf("table '%s' not found", tableName)
	}

	return info, nil
}

// NewMCPServer creates an MCP server with database tools.
func NewMCPServer(cfg Config) (*mcp.Server, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}

	server := mcp.NewServer("database", "1.0.0")

	// Query Tool
	server.RegisterTool(mcp.Tool{
		Name:        "db_query",
		Description: "Execute a SQL SELECT query and return results",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"query": {Type: "string", Description: "SQL SELECT query to execute"},
			},
			Required: []string{"query"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		query, _ := params["query"].(string)
		if query == "" {
			return mcp.ErrorResult(fmt.Errorf("query is required")), nil
		}

		result, err := client.Query(ctx, query)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(result)
	})

	// Exec Tool (only if not read-only)
	if !cfg.ReadOnly {
		server.RegisterTool(mcp.Tool{
			Name:        "db_exec",
			Description: "Execute a SQL INSERT, UPDATE, or DELETE query",
			InputSchema: mcp.InputSchema{
				Type: "object",
				Properties: map[string]mcp.Property{
					"query": {Type: "string", Description: "SQL query to execute (INSERT, UPDATE, DELETE)"},
				},
				Required: []string{"query"},
			},
		}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
			query, _ := params["query"].(string)
			if query == "" {
				return mcp.ErrorResult(fmt.Errorf("query is required")), nil
			}

			result, err := client.Exec(ctx, query)
			if err != nil {
				return mcp.ErrorResult(err), nil
			}

			return mcp.JSONResult(result)
		})
	}

	// List Tables Tool
	server.RegisterTool(mcp.Tool{
		Name:        "db_list_tables",
		Description: "List all tables in the database",
		InputSchema: mcp.InputSchema{
			Type:       "object",
			Properties: map[string]mcp.Property{},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		tables, err := client.ListTables(ctx)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		// Filter by allowed tables if configured
		if len(client.allowedTables) > 0 {
			var filtered []string
			for _, t := range tables {
				if client.allowedTables[strings.ToLower(t)] {
					filtered = append(filtered, t)
				}
			}
			tables = filtered
		}

		return mcp.JSONResult(map[string]interface{}{
			"tables": tables,
			"count":  len(tables),
		})
	})

	// Describe Table Tool
	server.RegisterTool(mcp.Tool{
		Name:        "db_describe_table",
		Description: "Get the schema/structure of a table",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"table": {Type: "string", Description: "Table name to describe"},
			},
			Required: []string{"table"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		tableName, _ := params["table"].(string)
		if tableName == "" {
			return mcp.ErrorResult(fmt.Errorf("table name is required")), nil
		}

		info, err := client.DescribeTable(ctx, tableName)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(info)
	})

	// Explain Query Tool
	server.RegisterTool(mcp.Tool{
		Name:        "db_explain",
		Description: "Explain the execution plan of a query",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"query":   {Type: "string", Description: "SQL query to explain"},
				"analyze": {Type: "boolean", Description: "Run EXPLAIN ANALYZE (actually executes the query)"},
			},
			Required: []string{"query"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		query, _ := params["query"].(string)
		if query == "" {
			return mcp.ErrorResult(fmt.Errorf("query is required")), nil
		}

		analyze, _ := params["analyze"].(bool)

		var explainQuery string
		if analyze {
			explainQuery = fmt.Sprintf("EXPLAIN ANALYZE %s", query)
		} else {
			explainQuery = fmt.Sprintf("EXPLAIN %s", query)
		}

		rows, err := client.db.QueryContext(ctx, explainQuery)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}
		defer rows.Close()

		var lines []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return mcp.ErrorResult(err), nil
			}
			lines = append(lines, line)
		}

		return mcp.SuccessResult(strings.Join(lines, "\n")), nil
	})

	// Sample Data Tool
	server.RegisterTool(mcp.Tool{
		Name:        "db_sample",
		Description: "Get a sample of rows from a table",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"table": {Type: "string", Description: "Table name to sample from"},
				"limit": {Type: "number", Description: "Number of rows to return (default: 10, max: 100)"},
			},
			Required: []string{"table"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		tableName, _ := params["table"].(string)
		if tableName == "" {
			return mcp.ErrorResult(fmt.Errorf("table name is required")), nil
		}

		// Check allowed tables
		if len(client.allowedTables) > 0 {
			if !client.allowedTables[strings.ToLower(tableName)] {
				return mcp.ErrorResult(fmt.Errorf("access to table '%s' is not allowed", tableName)), nil
			}
		}

		limit := 10
		if l, ok := params["limit"].(float64); ok && l > 0 {
			limit = int(l)
			if limit > 100 {
				limit = 100
			}
		}

		query := fmt.Sprintf("SELECT * FROM %s LIMIT %d", tableName, limit)
		result, err := client.Query(ctx, query)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(result)
	})

	// Register resources for table schemas
	tables, err := client.ListTables(context.Background())
	if err == nil {
		for _, table := range tables {
			// Skip if not in allowed tables
			if len(client.allowedTables) > 0 && !client.allowedTables[strings.ToLower(table)] {
				continue
			}

			server.RegisterResource(mcp.Resource{
				URI:         fmt.Sprintf("db://tables/%s", table),
				Name:        table,
				Description: fmt.Sprintf("Schema for table %s", table),
				MimeType:    "application/json",
			})
		}
	}

	// Set resource handler
	server.SetResourceHandler(func(ctx context.Context, uri string) (*mcp.ResourceContent, error) {
		if strings.HasPrefix(uri, "db://tables/") {
			tableName := strings.TrimPrefix(uri, "db://tables/")
			info, err := client.DescribeTable(ctx, tableName)
			if err != nil {
				return nil, err
			}

			data, _ := json.MarshalIndent(info, "", "  ")
			return &mcp.ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(data),
			}, nil
		}
		return nil, fmt.Errorf("unknown resource: %s", uri)
	})

	return server, nil
}
