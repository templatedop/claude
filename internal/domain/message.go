package domain

import (
	"encoding/json"
	"time"
)

// MessageType represents the type of inter-agent message.
type MessageType string

const (
	MessageTypeTaskAssignment   MessageType = "task_assignment"
	MessageTypeTaskComplete     MessageType = "task_complete"
	MessageTypeTaskFailed       MessageType = "task_failed"
	MessageTypeStatusUpdate     MessageType = "status_update"
	MessageTypeDataShare        MessageType = "data_share"
	MessageTypeQuery            MessageType = "query"
	MessageTypeQueryResponse    MessageType = "query_response"
	MessageTypeSignal           MessageType = "signal"
	MessageTypeBroadcast        MessageType = "broadcast"
	MessageTypeHeartbeat        MessageType = "heartbeat"
)

// Message represents a message exchanged between agents.
type Message struct {
	ID          string                 `json:"id"`
	Type        MessageType            `json:"type"`
	FromAgentID string                 `json:"from_agent_id"`
	ToAgentID   string                 `json:"to_agent_id"`
	Subject     string                 `json:"subject"`
	Payload     map[string]interface{} `json:"payload"`
	Timestamp   time.Time              `json:"timestamp"`
	CorrelationID string               `json:"correlation_id,omitempty"`
	ReplyTo     string                 `json:"reply_to,omitempty"`
	TTL         time.Duration          `json:"ttl,omitempty"`
}

// NewMessage creates a new message.
func NewMessage(msgType MessageType, from, to string, payload map[string]interface{}) *Message {
	return &Message{
		ID:        generateID(),
		Type:      msgType,
		FromAgentID: from,
		ToAgentID: to,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

// TaskAssignmentSignal is sent when assigning a task to an agent.
type TaskAssignmentSignal struct {
	Task     Task   `json:"task"`
	Priority int    `json:"priority"`
	Deadline *time.Time `json:"deadline,omitempty"`
}

// TaskCompleteSignal is sent when a task is completed.
type TaskCompleteSignal struct {
	TaskID    string                 `json:"task_id"`
	AgentID   string                 `json:"agent_id"`
	Result    TaskResult             `json:"result"`
	Artifacts []Artifact             `json:"artifacts"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// TaskFailedSignal is sent when a task fails.
type TaskFailedSignal struct {
	TaskID     string `json:"task_id"`
	AgentID    string `json:"agent_id"`
	Error      string `json:"error"`
	Retryable  bool   `json:"retryable"`
	RetryAfter time.Duration `json:"retry_after,omitempty"`
}

// StatusUpdateSignal is sent to update agent status.
type StatusUpdateSignal struct {
	AgentID   string      `json:"agent_id"`
	Status    AgentStatus `json:"status"`
	Message   string      `json:"message,omitempty"`
	Progress  float64     `json:"progress,omitempty"` // 0.0 to 1.0
	Metrics   AgentMetrics `json:"metrics,omitempty"`
}

// DataShareSignal is sent to share data between agents.
type DataShareSignal struct {
	FromAgentID string                 `json:"from_agent_id"`
	DataKey     string                 `json:"data_key"`
	DataType    string                 `json:"data_type"`
	Data        map[string]interface{} `json:"data"`
	Persistent  bool                   `json:"persistent"`
}

// QuerySignal is sent to query another agent.
type QuerySignal struct {
	QueryID   string                 `json:"query_id"`
	FromAgent string                 `json:"from_agent"`
	Query     string                 `json:"query"`
	Params    map[string]interface{} `json:"params,omitempty"`
}

// QueryResponseSignal is the response to a query.
type QueryResponseSignal struct {
	QueryID  string                 `json:"query_id"`
	ToAgent  string                 `json:"to_agent"`
	Response map[string]interface{} `json:"response"`
	Error    string                 `json:"error,omitempty"`
}

// BroadcastSignal is sent to all agents.
type BroadcastSignal struct {
	FromAgentID string                 `json:"from_agent_id"`
	Topic       string                 `json:"topic"`
	Message     string                 `json:"message"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// HeartbeatSignal is sent periodically to indicate agent is alive.
type HeartbeatSignal struct {
	AgentID   string       `json:"agent_id"`
	Status    AgentStatus  `json:"status"`
	Load      float64      `json:"load"` // 0.0 to 1.0
	TaskCount int          `json:"task_count"`
	Metrics   AgentMetrics `json:"metrics"`
}

// SignalChannel names for Temporal workflows.
const (
	SignalChannelTaskAssignment = "task_assignment"
	SignalChannelTaskComplete   = "task_complete"
	SignalChannelTaskFailed     = "task_failed"
	SignalChannelStatusUpdate   = "status_update"
	SignalChannelDataShare      = "data_share"
	SignalChannelQuery          = "query"
	SignalChannelQueryResponse  = "query_response"
	SignalChannelBroadcast      = "broadcast"
	SignalChannelHeartbeat      = "heartbeat"
	SignalChannelShutdown       = "shutdown"
)

// ToJSON converts a message to JSON bytes.
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON parses a message from JSON bytes.
func (m *Message) FromJSON(data []byte) error {
	return json.Unmarshal(data, m)
}

// Simple ID generator (in production, use UUID)
var idCounter int64

func generateID() string {
	idCounter++
	return time.Now().Format("20060102150405") + "-" + string(rune('A'+idCounter%26))
}
