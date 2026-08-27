package bootstrap

import (
	"context"
	"io"
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
	"github.com/t33n-software/git-governance/internal/adapters/report"
	"github.com/t33n-software/git-governance/internal/adapters/system"
	"github.com/t33n-software/git-governance/internal/adapters/terminal"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/policy"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/workflow"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

type appOptions struct {
	interactive         string
	output              string
	quiet               bool
	color               string
	accessible          bool
	remote              string
	repository          string
	config              string
	qualityConfig       string
	pullRequestProvider string
	dryRun              bool
	yes                 bool
	timeout             time.Duration
}

type Runtime struct {
	GitFactory                        func(timeout time.Duration) port.GitRepository
	StoreFactory                      func(path string) port.PreferencesStore
	KeyPolicy                         port.KeyPolicy
	Quality                           port.QualityRunner
	QualityFactory                    func(path string, timeout time.Duration) port.QualityRunner
	HotfixRecords                     port.HotfixReleaseRecordStore
	Publisher                         port.PullRequestPublisher
	GitHubAuthFactory                 func(timeout time.Duration) github.AuthProvider
	Browser                           browser.Opener
	GitHubCredentialBrokerURL         func() string
	GitHubWorkloadIdentity            func() string
	GitHubWorkflowToken               func() string
	GitHubWorkflowTokenEnabled        func() bool
	HotfixPropagationPublisherEnabled func() bool
	Tools                             port.ToolInspector
	PromptFactory                     func(accessible bool, color string) port.Prompt
	InputIsTerminal                   func() bool
	OutputIsTerminal                  func() bool
}

type application struct {
	runtime Runtime
	options *appOptions
}

type services struct {
	git         port.GitRepository
	branches    *branchapp.Service
	sync        *branchapp.Synchronizer
	scratch     *branchapp.ScratchMerger
	commits     *commitapp.Service
	tickets     *workflow.TicketService
	releases    *workflow.ReleaseService
	lifecycle   port.ReleaseLifecycleProvider
	preferences *policy.PreferencesService
	doctor      *policy.DoctorService
	githubAuth  github.AuthProvider
}

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

func (application *application) services() services {
	git := application.runtime.GitFactory(application.options.timeout)
	store := application.runtime.StoreFactory(application.options.config)
	qualityRunner := application.runtime.Quality
	if qualityRunner == nil {
		qualityRunner = application.runtime.QualityFactory(application.options.qualityConfig, application.options.timeout)
	}
	githubAuth := application.runtime.GitHubAuthFactory(application.options.timeout)
	credentialResolver := github.CredentialResolver(githubAuth)
	if application.runtime.GitHubWorkflowTokenEnabled() {
		credentialResolver = github.NewWorkflowTokenResolver(application.runtime.GitHubWorkflowToken())
	} else if brokerURL := strings.TrimSpace(application.runtime.GitHubCredentialBrokerURL()); brokerURL != "" {
		credentialResolver = github.NewBrokerResolver(github.BrokerOptions{
			Endpoint:         brokerURL,
			WorkloadIdentity: application.runtime.GitHubWorkloadIdentity,
			HTTPClient:       &http.Client{Timeout: application.options.timeout},
		})
	}
	publisher := application.runtime.Publisher
	if publisher == nil && application.options.pullRequestProvider == "github" {
		publisher = github.New(github.Options{
			Resolver: credentialResolver,
			Timeout:  application.options.timeout,
		})
	}
	branches := branchapp.NewService(git, application.runtime.KeyPolicy)
	finalQuality := branchapp.NewFinalQualityGate(git, qualityRunner)
	sync := branchapp.NewSynchronizer(git, branches, qualityRunner)
	scratch := branchapp.NewScratchMerger(git, branches)
	commits := commitapp.NewService(git, application.runtime.KeyPolicy, sync)
	tickets := workflow.NewTicketService(branches, sync, git, qualityRunner, publisher).
		WithScratchMerger(scratch)
	if finalQuality.Available() {
		sync.WithFinalQualityGate(finalQuality)
		tickets.WithFinalQualityGate(finalQuality)
	}
	lifecycle, _ := publisher.(port.ReleaseLifecycleProvider)
	protectedRequests, _ := publisher.(port.ProtectedLineRequestProvider)
	hotfixLifecycle, _ := publisher.(port.MainHotfixLifecycleProvider)
	releases := workflow.NewReleaseService(branches, git, publisher).
		WithTicketService(tickets).
		WithQualityRunner(qualityRunner).
		WithHotfixReleaseRecordStore(application.runtime.HotfixRecords).
		WithHotfixManifestPublication(application.runtime.HotfixPropagationPublisherEnabled()).
		WithMainHotfixLifecycleProvider(hotfixLifecycle).
		WithReleaseLifecycleProvider(lifecycle).
		WithProtectedLineRequestProvider(protectedRequests)
	policyInspector, _ := application.runtime.KeyPolicy.(port.PolicyInspector)
	return services{
		git:         git,
		branches:    branches,
		sync:        sync,
		scratch:     scratch,
		commits:     commits,
		tickets:     tickets,
		releases:    releases,
		lifecycle:   lifecycle,
		preferences: policy.NewPreferencesService(store),
		doctor:      policy.NewDoctorServiceWithDependencies(git, store, policyInspector, application.runtime.Tools),
		githubAuth:  githubAuth,
	}
}

func (application *application) repository(ctx context.Context) (port.RepositoryIdentity, error) {
	identity, err := application.services().git.Discover(ctx, application.options.repository)
	if err != nil {
		return port.RepositoryIdentity{}, err
	}
	identity.Remote = application.options.remote
	return identity, nil
}

func (application *application) reporter(writer io.Writer) port.Reporter {
	if writer == nil {
		writer = os.Stdout
	}
	format, _ := application.outputFormat()
	return report.New(report.Options{
		Writer: writer,
		Format: format,
		Quiet:  application.options.quiet,
		Color:  application.colorEnabled(),
	})
}

func (application *application) outputFormat() (report.Format, error) {
	switch application.options.output {
	case "", "human":
		return report.FormatHuman, nil
	case "json":
		return report.FormatJSON, nil
	default:
		return "", problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "output",
			Actual:      application.options.output,
			Expected:    "human or json",
			Rule:        "output format must be explicit and stable",
			Example:     "--output json",
			Remediation: "select human or json output",
		})
	}
}

func (application *application) promptAvailable() bool {
	if application.options.output == "json" || application.options.interactive == "never" {
		return false
	}
	if application.options.interactive == "always" {
		return application.inputIsTerminal() && application.outputIsTerminal()
	}
	return application.inputIsTerminal() && application.outputIsTerminal()
}

func (application *application) prompt() port.Prompt {
	return application.runtime.PromptFactory(application.options.accessible, application.options.color)
}

func (application *application) inputIsTerminal() bool {
	return application.runtime.InputIsTerminal != nil && application.runtime.InputIsTerminal()
}

func (application *application) outputIsTerminal() bool {
	return application.runtime.OutputIsTerminal != nil && application.runtime.OutputIsTerminal()
}

func (application *application) colorEnabled() bool {
	if application.options.output != "human" {
		return false
	}
	switch application.options.color {
	case "always":
		return true
	case "never":
		return false
	default:
		return application.outputIsTerminal()
	}
}

func (application *application) requireInput(
	ctx context.Context,
	value, label, description string,
	validators ...port.InputValidator,
) (string, error) {
	if value != "" {
		return value, nil
	}
	if !application.promptAvailable() {
		return "", missingInput(label)
	}
	var validate port.InputValidator
	if len(validators) > 0 {
		validate = validators[0]
	}
	return application.prompt().Input(ctx, port.InputRequest{
		Label:       label,
		Description: description,
		Required:    true,
		Validate:    validate,
	})
}

func (application *application) optionalConfirmation(ctx context.Context, label, description string, defaultValue bool) (bool, error) {
	if application.options.yes {
		return true, nil
	}
	if !application.promptAvailable() {
		return defaultValue, nil
	}
	return application.prompt().Confirm(ctx, port.ConfirmRequest{
		Label:       label,
		Description: description,
		Default:     defaultValue,
	})
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func stderrIsTerminal() bool {
	info, err := os.Stderr.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func missingInput(field string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeInvalidInput,
		Category:    problem.CategoryUsage,
		Field:       field,
		Expected:    "a value supplied by a flag or interactive terminal",
		Rule:        "non-interactive execution requires all mandatory values",
		Remediation: "supply the required flag or run in an interactive terminal",
	})
}
