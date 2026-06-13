package approval

// Command is an approval command. CommandType matches the contract command name.
type Command interface{ CommandType() string }

// Event is an approval domain event. EventType matches the catalog event type.
type Event interface{ EventType() string }

// --- Commands ---

type RequestApproval struct {
	ApprovalID string
	TaskID     string
	Kind       string
	Risk       string
	Context    map[string]any
}

type ApproveApproval struct {
	ApprovalID string
	ResolvedBy string
	Reason     string
}

type RejectApproval struct {
	ApprovalID string
	ResolvedBy string
	Reason     string
}

func (RequestApproval) CommandType() string { return "RequestApproval" }
func (ApproveApproval) CommandType() string { return "ApproveApproval" }
func (RejectApproval) CommandType() string  { return "RejectApproval" }

// --- Events --- (payloads mirror contracts/events/*.schema.json)

type ApprovalRequested struct {
	ApprovalID string
	TaskID     string
	Kind       string
	Risk       string
	Context    map[string]any
}

type ApprovalResolved struct {
	ApprovalID string
	TaskID     string
	Decision   string // approve | reject
	ResolvedBy string
	Reason     string
}

func (ApprovalRequested) EventType() string { return "ApprovalRequested" }
func (ApprovalResolved) EventType() string  { return "ApprovalResolved" }
