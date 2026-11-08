package bootstrap

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/t33n-software/git-governance/internal/adapters/browser"
	"github.com/t33n-software/git-governance/internal/adapters/configfs"
	"github.com/t33n-software/git-governance/internal/adapters/gitcli"
	"github.com/t33n-software/git-governance/internal/adapters/github"
	"github.com/t33n-software/git-governance/internal/adapters/hotfixrecord"
	"github.com/t33n-software/git-governance/internal/adapters/quality"
	"github.com/t33n-software/git-governance/internal/adapters/system"
	"github.com/t33n-software/git-governance/internal/adapters/terminal"
	"github.com/t33n-software/git-governance/internal/application/policy"
	"github.com/t33n-software/git-governance/internal/application/port"
)

// defaultRuntime constructs the production adapter surface. Every field is a
// boundary constructor: the Git transport, the preference store, the quality
// runner, the hotfix record store, the authentication provider, the browser,
// the server-side credential compositions, the tool inspector, and the
// terminal interaction surface. Environment-resolved compositions stay
// functions so the injected test runtime can replace each one independently.
func defaultRuntime() Runtime {
	return Runtime{
		GitFactory: func(timeout time.Duration) port.GitRepository {
			return gitcli.New(gitcli.Options{
				Timeout:              timeout,
				RequireSignedCommits: policy.Describe().CommitSigning == policy.CommitSigningRequired,
			})
		},
		StoreFactory: func(path string) port.PreferencesStore {
			return configfs.New(configfs.Options{Path: path})
		},
		KeyPolicy: policy.SyntaxOnlyKeyPolicy{},
		QualityFactory: func(path string, timeout time.Duration) port.QualityRunner {
			return quality.New(quality.Options{
				Path:           path,
				DefaultTimeout: timeout,
			})
		},
		HotfixRecords: hotfixrecord.New(),
		GitHubAuthFactory: func(timeout time.Duration) github.AuthProvider {
			return github.NewAuthService(github.AuthOptions{
				HTTPClient: &http.Client{Timeout: timeout},
			})
		},
		Browser: browser.New(browser.Options{}),
		GitHubCredentialBrokerURL: func() string {
			return os.Getenv("GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL")
		},
		GitHubWorkloadIdentity: func() string {
			return os.Getenv("GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN")
		},
		GitHubWorkflowToken: func() string {
			return os.Getenv("GITHUB_TOKEN")
		},
		GitHubWorkflowTokenEnabled: func() bool {
			return os.Getenv("GITHUB_ACTIONS") == "true" &&
				strings.TrimSpace(os.Getenv("GIT_GOVERNANCE_WORKFLOW_TOKEN")) == "server"
		},
		HotfixPropagationPublisherEnabled: func() bool {
			return strings.TrimSpace(os.Getenv("GIT_GOVERNANCE_HOTFIX_PROPAGATION_PUBLISHER")) == "server" &&
				strings.TrimSpace(os.Getenv("GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL")) != "" &&
				strings.TrimSpace(os.Getenv("GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN")) != ""
		},
		Tools: system.New(system.Options{}),
		PromptFactory: func(accessible bool, color string) port.Prompt {
			return terminal.New(terminal.Options{Accessible: accessible, Color: color})
		},
		InputIsTerminal:  stdinIsTerminal,
		OutputIsTerminal: stdoutIsTerminal,
	}
}
