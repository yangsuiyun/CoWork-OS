package task

// Command is a task command. CommandType matches the contract command name.
type Command interface{ CommandType() string }

// Event is a task domain event. EventType matches the catalog event type.
type Event interface{ EventType() string }

// --- Commands ---

type CreateTask struct {
	TaskID          string
	WorkspaceID     string
	Origin          string
	CanonicalPrompt string
	Risk            string
	IsChild         bool
	ParentTaskID    string
}

type PlanTask struct{ TaskID string }
type StartTurn struct {
	TaskID string
	TurnID string
}
type CompleteTask struct{ TaskID string }
type FailTask struct {
	TaskID    string
	ErrorCode string
	Message   string
}
type CancelTask struct {
	TaskID      string
	CancelledBy string
}
type AppendArtifact struct {
	TaskID     string
	ArtifactID string
	Path       string
	SHA256     string
	Mime       string
	Size       int64
}

func (CreateTask) CommandType() string      { return "CreateTask" }
func (PlanTask) CommandType() string        { return "PlanTask" }
func (StartTurn) CommandType() string       { return "StartTurn" }
func (CompleteTask) CommandType() string    { return "CompleteTask" }
func (FailTask) CommandType() string        { return "FailTask" }
func (CancelTask) CommandType() string     { return "CancelTask" }
func (AppendArtifact) CommandType() string { return "AppendArtifact" }

// --- Events --- (payloads mirror contracts/events/*.schema.json)

type TaskCreated struct {
	TaskID          string
	WorkspaceID     string
	Origin          string
	CanonicalPrompt string
	Risk            string
	ParentTaskID    string
}
type TaskPlanned struct{ TaskID string }
type TurnStarted struct {
	TaskID string
	TurnID string
}
type TurnCompleted struct {
	TaskID  string
	TurnID  string
	Outcome string
}
type TaskCompleted struct{ TaskID string }
type TaskFailed struct {
	TaskID    string
	ErrorCode string
	Message   string
}
type TaskCancelled struct {
	TaskID      string
	CancelledBy string
}
type ArtifactCreated struct {
	ArtifactID string
	TaskID     string
	Path       string
	SHA256     string
	Mime       string
	Size       int64
}

func (TaskCreated) EventType() string       { return "TaskCreated" }
func (TaskPlanned) EventType() string       { return "TaskPlanned" }
func (TurnStarted) EventType() string       { return "TurnStarted" }
func (TurnCompleted) EventType() string     { return "TurnCompleted" }
func (TaskCompleted) EventType() string     { return "TaskCompleted" }
func (TaskFailed) EventType() string        { return "TaskFailed" }
func (TaskCancelled) EventType() string   { return "TaskCancelled" }
func (ArtifactCreated) EventType() string { return "ArtifactCreated" }
