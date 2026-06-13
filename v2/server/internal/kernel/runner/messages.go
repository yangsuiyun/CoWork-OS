package runner

// Command is a runner command. CommandType matches the contract command name.
type Command interface{ CommandType() string }

// Event is a runner domain event. EventType matches the catalog event type.
type Event interface{ EventType() string }

// --- Commands ---

type RegisterRunner struct {
	RunnerID     string
	WorkspaceID  string
	Capabilities []string
}

type RunnerHeartbeat struct {
	RunnerID string
	Pulse    int
}

type MarkRunnerStale struct {
	RunnerID string
	Reason   string
}

func (RegisterRunner) CommandType() string  { return "RegisterRunner" }
func (RunnerHeartbeat) CommandType() string { return "RunnerHeartbeat" }
func (MarkRunnerStale) CommandType() string { return "MarkRunnerStale" }

// --- Events --- (payloads mirror contracts/events/*.schema.json)

type RunnerRegistered struct {
	RunnerID     string
	WorkspaceID  string
	Capabilities []string
}

type RunnerHeartbeatPulsed struct {
	RunnerID string
	Pulse    int
}

type RunnerStale struct {
	RunnerID string
	Reason   string
}

func (RunnerRegistered) EventType() string      { return "RunnerRegistered" }
func (RunnerHeartbeatPulsed) EventType() string { return "RunnerHeartbeat" }
func (RunnerStale) EventType() string           { return "RunnerStale" }
