package facts

// Package facts contains the payload-free normalized representation exchanged
// between source adapters and durable storage. Raw source content never appears
// in these values.

type Usage struct {
	Input, CachedInput, Output, ReasoningOutput, Total *int64
}

type Source struct {
	ID, SessionID, SegmentID, SegmentFingerprint string
	Kind, Path, DetectedVersion, AdapterVersion  string
	Size, MTimeNS, CompleteOffset                int64
	CompleteOrdinal, PendingTail                 int
	PrefixSHA256, State, StateReason             string
}

type Project struct {
	ID, Kind, Identity, DisplayName string
	Aliases                         map[string]string
}

type Session struct {
	ID, SourceSessionID, ProjectID, RootWorkUnitID string
	Purpose, Source, Originator, Title             string
	StartedAt, EndedAt, LineageCoverage            string
}

type Segment struct {
	ID, SessionID, SourceID, Fingerprint string
	StartedAt, SourceOrderKey            string
	Ordinal                              int
}

type Lineage struct {
	ParentSessionID, ChildSessionID, Kind, SpawningTurnID, SourceEventID string
}

type Turn struct {
	ID, SessionID, SourceTurnID, State, StartedAt, TerminalAt, CompletedAt string
	SourceOrderKey                                                         string
	Model, ReasoningEffort, NormalizationKind                              string
	Ordinal                                                                int
	Usage                                                                  *Usage
	Cumulative                                                             *Usage
}

type Event struct {
	ID, SegmentID, TurnID, SemanticPhase, RecordType, Kind, ObservedAt string
	SourceOrderKey                                                     string
	RecordOrdinal                                                      int
	ByteStart, ByteEnd, PayloadLength                                  int64
	ContentSHA256, AdapterVersion                                      string
	SearchText, SearchCategory                                         string
}

type Message struct {
	EventID, Role, Phase, SourceMessageID, ContentSHA256 string
	Readable                                             bool
	ContentLength                                        int64
}

type ToolCall struct {
	ID, SessionID, TurnID, SourceCallID, Phase, EventID string
	Name, Family, Status                                string
	ExitCode, DurationMS                                *int64
}

type Capacity struct {
	ID, EventID, LimitID, ObservedAt, ResetsAt string
	WindowMinutes                              *int64
	UsedPercent, RemainingPercent              *float64
}

type Compaction struct {
	EventID, TurnID, TriggerKind, SummarySHA256 string
	SummaryLength                               *int64
	FirstWindowID, PreviousWindowID, WindowID   string
	WindowNumber                                int
}

type Evidence struct {
	ID, EventID, SourceID, EventFingerprint, SourceFingerprint string
	Availability                                               string
}

type Coverage struct {
	ScopeKind, ScopeID, FieldKey, Fidelity, Reason string
	Observed, Eligible                             int
}

type Batch struct {
	Source        Source
	Project       *Project
	Session       *Session
	Segment       *Segment
	Lineage       []Lineage
	Turns         []Turn
	Events        []Event
	Messages      []Message
	Tools         []ToolCall
	Capacity      []Capacity
	Compactions   []Compaction
	Evidence      []Evidence
	Coverage      []Coverage
	SessionLabels map[string]string
}
