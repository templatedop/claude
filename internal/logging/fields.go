package logging

import (
	"context"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Field represents a structured log field
type Field struct {
	Key   string
	Value interface{}
	Type  FieldType
}

// FieldType represents the type of a log field
type FieldType int

const (
	FieldTypeAny FieldType = iota
	FieldTypeString
	FieldTypeInt
	FieldTypeInt64
	FieldTypeFloat64
	FieldTypeBool
	FieldTypeDuration
	FieldTypeTime
	FieldTypeError
	FieldTypeStringer
)

// Common field constructors

// String creates a string field
func String(key, value string) Field {
	return Field{Key: key, Value: value, Type: FieldTypeString}
}

// Int creates an int field
func Int(key string, value int) Field {
	return Field{Key: key, Value: value, Type: FieldTypeInt}
}

// Int64 creates an int64 field
func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value, Type: FieldTypeInt64}
}

// Float64 creates a float64 field
func Float64(key string, value float64) Field {
	return Field{Key: key, Value: value, Type: FieldTypeFloat64}
}

// Bool creates a bool field
func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value, Type: FieldTypeBool}
}

// Duration creates a duration field
func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value, Type: FieldTypeDuration}
}

// Time creates a time field
func Time(key string, value time.Time) Field {
	return Field{Key: key, Value: value, Type: FieldTypeTime}
}

// Error creates an error field
func Error(err error) Field {
	return Field{Key: "error", Value: err, Type: FieldTypeError}
}

// NamedError creates a named error field
func NamedError(key string, err error) Field {
	return Field{Key: key, Value: err, Type: FieldTypeError}
}

// Any creates a field with any value
func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value, Type: FieldTypeAny}
}

// Stringer creates a field from a fmt.Stringer
func Stringer(key string, value interface{ String() string }) Field {
	return Field{Key: key, Value: value, Type: FieldTypeStringer}
}

// Domain-specific field constructors

// WorkflowID creates a workflow ID field
func WorkflowID(id string) Field {
	return String("workflow_id", id)
}

// RunID creates a run ID field
func RunID(id string) Field {
	return String("run_id", id)
}

// ActivityName creates an activity name field
func ActivityName(name string) Field {
	return String("activity", name)
}

// AgentType creates an agent type field
func AgentType(t string) Field {
	return String("agent_type", t)
}

// TaskID creates a task ID field
func TaskID(id string) Field {
	return String("task_id", id)
}

// Namespace creates a namespace field
func Namespace(ns string) Field {
	return String("namespace", ns)
}

// Operation creates an operation field
func Operation(op string) Field {
	return String("operation", op)
}

// Component creates a component field
func Component(c string) Field {
	return String("component", c)
}

// RequestID creates a request ID field
func RequestID(id string) Field {
	return String("request_id", id)
}

// UserID creates a user ID field
func UserID(id string) Field {
	return String("user_id", id)
}

// Path creates a file path field
func Path(p string) Field {
	return String("path", p)
}

// Bucket creates a storage bucket field
func Bucket(b string) Field {
	return String("bucket", b)
}

// Count creates a count field
func Count(n int) Field {
	return Int("count", n)
}

// Size creates a size field (bytes)
func Size(n int64) Field {
	return Int64("size_bytes", n)
}

// Latency creates a latency field
func Latency(d time.Duration) Field {
	return Duration("latency", d)
}

// StatusCode creates an HTTP status code field
func StatusCode(code int) Field {
	return Int("status_code", code)
}

// Method creates an HTTP method field
func Method(m string) Field {
	return String("method", m)
}

// URL creates a URL field
func URL(u string) Field {
	return String("url", u)
}

// Convert fields to zap fields
func fieldsToZap(fields []Field) []zap.Field {
	zapFields := make([]zap.Field, len(fields))
	for i, f := range fields {
		zapFields[i] = fieldToZap(f)
	}
	return zapFields
}

func fieldToZap(f Field) zap.Field {
	switch f.Type {
	case FieldTypeString:
		return zap.String(f.Key, f.Value.(string))
	case FieldTypeInt:
		return zap.Int(f.Key, f.Value.(int))
	case FieldTypeInt64:
		return zap.Int64(f.Key, f.Value.(int64))
	case FieldTypeFloat64:
		return zap.Float64(f.Key, f.Value.(float64))
	case FieldTypeBool:
		return zap.Bool(f.Key, f.Value.(bool))
	case FieldTypeDuration:
		return zap.Duration(f.Key, f.Value.(time.Duration))
	case FieldTypeTime:
		return zap.Time(f.Key, f.Value.(time.Time))
	case FieldTypeError:
		if err, ok := f.Value.(error); ok {
			return zap.Error(err)
		}
		return zap.Skip()
	case FieldTypeStringer:
		if s, ok := f.Value.(interface{ String() string }); ok {
			return zap.Stringer(f.Key, s)
		}
		return zap.Any(f.Key, f.Value)
	default:
		return zap.Any(f.Key, f.Value)
	}
}

func fieldsToInterface(fields []Field) []interface{} {
	result := make([]interface{}, 0, len(fields)*2)
	for _, f := range fields {
		result = append(result, f.Key, f.Value)
	}
	return result
}

// Context key types
type contextKey string

const (
	contextKeyWorkflowID contextKey = "workflow_id"
	contextKeyRunID      contextKey = "run_id"
	contextKeyRequestID  contextKey = "request_id"
	contextKeyUserID     contextKey = "user_id"
	contextKeyAgentType  contextKey = "agent_type"
	contextKeyTaskID     contextKey = "task_id"
)

// Context helpers

// WithWorkflowID adds workflow ID to context
func WithWorkflowID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKeyWorkflowID, id)
}

// WithRunID adds run ID to context
func WithRunID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKeyRunID, id)
}

// WithRequestID adds request ID to context
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKeyRequestID, id)
}

// WithUserID adds user ID to context
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKeyUserID, id)
}

// WithAgentType adds agent type to context
func WithAgentType(ctx context.Context, t string) context.Context {
	return context.WithValue(ctx, contextKeyAgentType, t)
}

// WithTaskID adds task ID to context
func WithTaskID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKeyTaskID, id)
}

// extractContextFields extracts all logging fields from context
func extractContextFields(ctx context.Context) []Field {
	var fields []Field

	if v := ctx.Value(contextKeyWorkflowID); v != nil {
		fields = append(fields, WorkflowID(v.(string)))
	}
	if v := ctx.Value(contextKeyRunID); v != nil {
		fields = append(fields, RunID(v.(string)))
	}
	if v := ctx.Value(contextKeyRequestID); v != nil {
		fields = append(fields, RequestID(v.(string)))
	}
	if v := ctx.Value(contextKeyUserID); v != nil {
		fields = append(fields, UserID(v.(string)))
	}
	if v := ctx.Value(contextKeyAgentType); v != nil {
		fields = append(fields, AgentType(v.(string)))
	}
	if v := ctx.Value(contextKeyTaskID); v != nil {
		fields = append(fields, TaskID(v.(string)))
	}

	return fields
}

// ContextWithFields returns a context with logging fields
func ContextWithFields(ctx context.Context, fields ...Field) context.Context {
	for _, f := range fields {
		switch f.Key {
		case "workflow_id":
			ctx = context.WithValue(ctx, contextKeyWorkflowID, f.Value)
		case "run_id":
			ctx = context.WithValue(ctx, contextKeyRunID, f.Value)
		case "request_id":
			ctx = context.WithValue(ctx, contextKeyRequestID, f.Value)
		case "user_id":
			ctx = context.WithValue(ctx, contextKeyUserID, f.Value)
		case "agent_type":
			ctx = context.WithValue(ctx, contextKeyAgentType, f.Value)
		case "task_id":
			ctx = context.WithValue(ctx, contextKeyTaskID, f.Value)
		}
	}
	return ctx
}

// Object creates a field that marshals an object as JSON
func Object(key string, val zapcore.ObjectMarshaler) Field {
	return Field{Key: key, Value: val, Type: FieldTypeAny}
}

// Strings creates a field with a string slice
func Strings(key string, values []string) Field {
	return Field{Key: key, Value: values, Type: FieldTypeAny}
}

// Ints creates a field with an int slice
func Ints(key string, values []int) Field {
	return Field{Key: key, Value: values, Type: FieldTypeAny}
}
