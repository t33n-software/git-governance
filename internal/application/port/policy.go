package port

import (
	"context"

	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

// KeyPolicy validates a syntactically valid key against the active local
// policy. The first implementation only checks syntax; a bundle adapter can
// add repository authorization later.
type KeyPolicy interface {
	ValidateKey(ctx context.Context, repository RepositoryIdentity, key ticket.Key) error
}

// PolicyStatus describes the active local policy mode without exposing policy
// implementation details to diagnostics consumers.
type PolicyStatus struct {
	Mode          string
	BundlePresent bool
	BundleFresh   bool
	Detail        string
}

// PolicyInspector reports read-only status for the active policy adapter.
type PolicyInspector interface {
	Status(ctx context.Context, repository RepositoryIdentity) (PolicyStatus, error)
}

// ToolInspector performs bounded read-only diagnostics for external tools and
// repository-local configuration files.
type ToolInspector interface {
	Platform() (operatingSystem string, architecture string)
	Version(ctx context.Context, executable string) (string, error)
	FileExists(path string) (bool, error)
}

// Preferences are user-scoped UX preferences, never organizational policy or
// secrets.
type Preferences struct {
	SchemaVersion int
	KnownKeys     []ticket.Key
	DefaultKey    *ticket.Key
	Accessible    bool
}

// PreferencesStore persists user-scoped preferences.
type PreferencesStore interface {
	Load(ctx context.Context) (Preferences, error)
	Save(ctx context.Context, preferences Preferences) error
}

// Prompt is the inbound interactive terminal boundary.
type Prompt interface {
	Input(ctx context.Context, request InputRequest) (string, error)
	Select(ctx context.Context, request SelectRequest) (string, error)
	Confirm(ctx context.Context, request ConfirmRequest) (bool, error)
}

// InputValidator validates one interactive input candidate. It returns an
// error that the terminal adapter can render before asking again.
type InputValidator func(string) error

// InputRequest describes an explanatory text input.
type InputRequest struct {
	Label       string
	Description string
	Default     string
	Required    bool
	Validate    InputValidator
	Sensitive   bool
}

// SelectRequest describes an explanatory single-value choice.
type SelectRequest struct {
	Label       string
	Description string
	Options     []SelectOption
	Default     string
}

// SelectOption is a stable machine value and a human-facing explanation.
type SelectOption struct {
	Value       string
	Label       string
	Description string
}

// ConfirmRequest describes a consequential action requiring explicit consent.
type ConfirmRequest struct {
	Label       string
	Description string
	Default     bool
}
