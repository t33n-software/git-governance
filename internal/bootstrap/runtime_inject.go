package bootstrap

import (
	"github.com/t33n-software/git-governance/internal/application/policy"
)

// newApplication binds the injected runtime to the resolved global options and
// restores the production default for every boundary the caller left unset.
// The fallback reads the production default surface once per unset boundary,
// so an injected runtime never has to supply a complete adapter set.
func newApplication(runtime Runtime, options *appOptions) *application {
	if runtime.GitFactory == nil {
		runtime.GitFactory = defaultRuntime().GitFactory
	}
	if runtime.StoreFactory == nil {
		runtime.StoreFactory = defaultRuntime().StoreFactory
	}
	if runtime.KeyPolicy == nil {
		runtime.KeyPolicy = policy.SyntaxOnlyKeyPolicy{}
	}
	if runtime.Quality == nil && runtime.QualityFactory == nil {
		runtime.QualityFactory = defaultRuntime().QualityFactory
	}
	if runtime.HotfixRecords == nil {
		runtime.HotfixRecords = defaultRuntime().HotfixRecords
	}
	if runtime.GitHubAuthFactory == nil {
		runtime.GitHubAuthFactory = defaultRuntime().GitHubAuthFactory
	}
	if runtime.Browser == nil {
		runtime.Browser = defaultRuntime().Browser
	}
	if runtime.GitHubCredentialBrokerURL == nil {
		runtime.GitHubCredentialBrokerURL = defaultRuntime().GitHubCredentialBrokerURL
	}
	if runtime.GitHubWorkloadIdentity == nil {
		runtime.GitHubWorkloadIdentity = defaultRuntime().GitHubWorkloadIdentity
	}
	if runtime.GitHubWorkflowToken == nil {
		runtime.GitHubWorkflowToken = defaultRuntime().GitHubWorkflowToken
	}
	if runtime.GitHubWorkflowTokenEnabled == nil {
		runtime.GitHubWorkflowTokenEnabled = defaultRuntime().GitHubWorkflowTokenEnabled
	}
	if runtime.HotfixPropagationPublisherEnabled == nil {
		runtime.HotfixPropagationPublisherEnabled = defaultRuntime().HotfixPropagationPublisherEnabled
	}
	if runtime.Tools == nil {
		runtime.Tools = defaultRuntime().Tools
	}
	if runtime.PromptFactory == nil {
		runtime.PromptFactory = defaultRuntime().PromptFactory
	}
	if runtime.InputIsTerminal == nil {
		runtime.InputIsTerminal = defaultRuntime().InputIsTerminal
	}
	if runtime.OutputIsTerminal == nil {
		runtime.OutputIsTerminal = defaultRuntime().OutputIsTerminal
	}
	return &application{runtime: runtime, options: options}
}
