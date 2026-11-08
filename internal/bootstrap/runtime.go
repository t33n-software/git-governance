package bootstrap

import (
	"context"
	"time"

	"github.com/t33n-software/git-governance/internal/adapters/browser"
	"github.com/t33n-software/git-governance/internal/adapters/github"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/policy"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/ticketalloc"
	"github.com/t33n-software/git-governance/internal/application/workflow"
)

// This file carries the composition contract of the runtime: the resolved
// global options, the injectable adapter surface, the application binding, and
// the composed service surface. The default adapter construction, the
// dependency injection, the service composition, the report and option
// context, and the interactive helpers live in the companion runtime files.

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
	// commandContext is bound by the framework's PersistentPreRunE before any
	// endpoint runs and lets the composition root resolve the selected remote
	// with the live command cancellation instead of an unbound background
	// context.
	commandContext context.Context
	// publisherResolved and publisherMemo cache the derived-publisher verdict
	// for one application instance, so the remote resolution runs at most
	// once per process regardless of how often the service graph is composed.
	publisherResolved bool
	publisherMemo     port.PullRequestPublisher
}

type services struct {
	git                port.GitRepository
	branches           *branchapp.Service
	sync               *branchapp.Synchronizer
	refreshSharedLines *branchapp.SharedLineRefresher
	scratch            *branchapp.ScratchMerger
	commits            *commitapp.Service
	tickets            *workflow.TicketService
	worktrees          *workflow.WorktreeService
	branchHygiene      *workflow.BranchHygieneService
	releases           *workflow.ReleaseService
	bootstrap          *workflow.BootstrapService
	allocation         *ticketalloc.Service
	lifecycle          port.ReleaseLifecycleProvider
	preferences        *policy.PreferencesService
	doctor             *policy.DoctorService
	githubAuth         github.AuthProvider
}
