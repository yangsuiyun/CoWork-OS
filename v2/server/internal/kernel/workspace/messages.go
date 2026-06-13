package workspace

// Command is a workspace command. CommandType matches the contract command name.
type Command interface{ CommandType() string }

// Event is a workspace domain event. EventType matches the catalog event type.
type Event interface{ EventType() string }

// Permissions is the workspace capability scope (spec 7.2: capability-first).
type Permissions struct {
	Paths   []string
	Domains []string
}

// --- Commands ---

type CreateWorkspace struct {
	WorkspaceID string
	Name        string
}

type UpdatePermissions struct {
	WorkspaceID string
	Permissions Permissions
}

func (CreateWorkspace) CommandType() string   { return "CreateWorkspace" }
func (UpdatePermissions) CommandType() string { return "UpdatePermissions" }

// --- Events --- (payloads mirror contracts/events/*.schema.json)

type WorkspaceCreated struct {
	WorkspaceID string
	Name        string
}

type PermissionsChanged struct {
	WorkspaceID string
	Version     int
	Permissions Permissions
}

func (WorkspaceCreated) EventType() string   { return "WorkspaceCreated" }
func (PermissionsChanged) EventType() string { return "PermissionsChanged" }
