package bootstrap

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/t33n-software/git-governance/internal/adapters/github"
	branchapp "github.com/t33n-software/git-governance/internal/application/branch"
	commitapp "github.com/t33n-software/git-governance/internal/application/commit"
	"github.com/t33n-software/git-governance/internal/application/policy"
	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/application/ticketalloc"
	"github.com/t33n-software/git-governance/internal/application/workflow"
)

// services composes the application services over the resolved adapters. The
// credential resolver chain prefers the protected workflow token, then the
// managed credential broker, then the local GitHub App authentication provider.
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
	// The allocation inventory is a derived read model over the resolved
	// adapter capabilities: the Git-transport surfaces always, the platform
	// surfaces exactly when a hosting provider is configured. A missing
	// required capability fails the inventory closed with the named
	// capability at scan time.
	localBranches, _ := git.(port.LocalBranchLister)
	remoteBranches, _ := git.(port.RemoteBranchLister)
	commitSubjects, _ := git.(port.CommitSubjectLister)
	hotfixRecords, _ := application.runtime.HotfixRecords.(port.HotfixReleaseRecordLister)
	worktreeLister, _ := git.(port.WorktreeInventoryLister)
	inventoryPullRequests, _ := publisher.(port.PullRequestInventoryLister)
	inventoryProtectedLineRequests, _ := publisher.(port.ProtectedLineRequestInventoryLister)
	appPermissions, _ := publisher.(port.AppPermissionInspector)
	allocation := ticketalloc.New(ticketalloc.Dependencies{
		LocalBranches:         localBranches,
		RemoteBranches:        remoteBranches,
		CommitSubjects:        commitSubjects,
		HotfixRecords:         hotfixRecords,
		Worktrees:             worktreeLister,
		RemoteURL:             git.RemoteURL,
		PullRequests:          inventoryPullRequests,
		ProtectedLineRequests: inventoryProtectedLineRequests,
		AppPermissions:        appPermissions,
	})
	branches := branchapp.NewService(git, application.runtime.KeyPolicy).WithTicketAllocation(allocation)
	finalQuality := branchapp.NewFinalQualityGate(git, qualityRunner)
	sync := branchapp.NewSynchronizer(git, branches, qualityRunner)
	scratch := branchapp.NewScratchMerger(git, branches)
	commits := commitapp.NewService(git, application.runtime.KeyPolicy, sync)
	// The post-publication return to the develop integration line is a local
	// operator-workspace behavior. Server-side controller compositions (the
	// protected workflow-token context and the dedicated hotfix propagation
	// publisher) run on ephemeral checkouts and never switch branches.
	integrationLineReturn := !application.runtime.GitHubWorkflowTokenEnabled() &&
		!application.runtime.HotfixPropagationPublisherEnabled()
	tickets := workflow.NewTicketService(branches, sync, git, qualityRunner, publisher).
		WithScratchMerger(scratch).
		WithIntegrationLineReturn(integrationLineReturn)
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
		WithTicketAllocation(allocation).
		WithHotfixManifestPublication(application.runtime.HotfixPropagationPublisherEnabled()).
		WithIntegrationLineReturn(integrationLineReturn).
		WithMainHotfixLifecycleProvider(hotfixLifecycle).
		WithReleaseLifecycleProvider(lifecycle).
		WithProtectedLineRequestProvider(protectedRequests)
	policyInspector, _ := application.runtime.KeyPolicy.(port.PolicyInspector)
	return services{
		git:                git,
		branches:           branches,
		sync:               sync,
		refreshSharedLines: branchapp.NewSharedLineRefresher(git),
		scratch:            scratch,
		commits:            commits,
		tickets:            tickets,
		releases:           releases,
		bootstrap: workflow.NewBootstrapService(branches, git, application.runtime.KeyPolicy, application.runtime.Tools).
			WithTicketAllocation(allocation).
			WithSigningReadiness(policy.SigningConfigurationProblem).
			WithPolicySnapshot(func() string {
				description := policy.Describe()
				return "schemaVersion=" + strconv.Itoa(description.SchemaVersion) +
					" keyPolicy=" + description.KeyPolicy +
					" commitSigning=" + description.CommitSigning
			}),
		lifecycle:     lifecycle,
		allocation:    allocation,
		worktrees:     workflow.NewWorktreeService(git).WithPullRequestInventory(inventoryPullRequests),
		branchHygiene: workflow.NewBranchHygieneService(git).WithPullRequestInventory(inventoryPullRequests),
		preferences:   policy.NewPreferencesService(store),
		doctor:        policy.NewDoctorServiceWithDependencies(git, store, policyInspector, application.runtime.Tools),
		githubAuth:    githubAuth,
	}
}
