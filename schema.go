package oltgo

import "time"

type Status string

const (
	StatusSuccess Status = "SUCCESS"
	StatusFailure Status = "FAILURE"
)

type CommunicationProto string

const (
	ProtoREST      CommunicationProto = "REST"
	ProtoGRPC      CommunicationProto = "gRPC"
	ProtoSOAP      CommunicationProto = "SOAP"
	ProtoWebSocket CommunicationProto = "WebSocket"
)

type EventType string

const (
	TypeFunction        EventType = "FUNCTION"
	TypeDatabase        EventType = "DATABASE"
	TypeExternalService EventType = "EXTERNAL_SERVICE"
	TypeInternalService EventType = "INTERNAL_SERVICE"
)

type Severity string

const (
	SeverityDebug Severity = "DEBUG"
	SeverityInfo  Severity = "INFO"
	SeverityWarn  Severity = "WARN"
	SeverityError Severity = "ERROR"
	SeverityFatal Severity = "FATAL"
)

type LogSchema struct {
	TracingID     string      `json:"tracing_id"`
	Status        Status      `json:"status"`
	Environment   Environment `json:"environment"`
	SchemaVersion string      `json:"schema_version"`
	Service       Service     `json:"service"`
	Request       *Request    `json:"request,omitempty"`
	Actor         *Actor      `json:"actor,omitempty"`
	Time          TimeInfo    `json:"time"`
	Events        []Event     `json:"events"`
}

type Request struct {
	Communication CommunicationProto `json:"communication"`
	Method        string             `json:"method,omitempty"`
	Route         string             `json:"route,omitempty"`
	Status        int                `json:"status"`
	UserAgent     string             `json:"user_agent,omitempty"`
}

type Actor struct {
	ID   string `json:"id"`
	Type string `json:"type"` // USER, SERVICE
	IP   string `json:"ip,omitempty"`
}

type TimeInfo struct {
	DurationMs int       `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
	FinishedAt time.Time `json:"finished_at"`
}

type Event struct {
	EventID       string         `json:"event_id"`
	ParentEventID *string        `json:"parent_event_id"`
	Name          string         `json:"name"`
	Type          EventType      `json:"type"`
	Status        Status         `json:"status"`
	Severity      Severity       `json:"severity"`
	Timestamp     time.Time      `json:"timestamp"`
	DurationMs    int            `json:"duration_ms"`
	Message       string         `json:"message"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	Errors        []EventError   `json:"errors,omitempty"`
}

type EventError struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Stack   *string `json:"stack,omitempty"`
}
