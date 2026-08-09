package packaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTagApprovalExplicitlyDispatchesReleaseArtifacts(t *testing.T) {
	t.Parallel()

	tagWorkflow := readWorkflow(t, "tag-approved-release.yml")
	for _, expected := range []string{
		"pull_request:",
		"branches:",
		"- main",
		"types:",
		"- closed",
		"startsWith(github.event.pull_request.head.ref, 'release/')",
		`git push origin "refs/tags/${TAG}"`,
		"actions: write",
		"actions/workflows/release.yml/dispatches",
		`\"inputs\":{\"tag\":\"${TAG}\"}`,
	} {
		if !strings.Contains(tagWorkflow, expected) {
			t.Fatalf("tag workflow does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"repository_dispatch",
	} {
		if strings.Contains(tagWorkflow, forbidden) {
			t.Fatalf("tag workflow must not contain %q", forbidden)
		}
	}

	releaseWorkflow := readWorkflow(t, "release.yml")
	for _, expected := range []string{
		"run-name: Release ${{ github.event_name == 'workflow_dispatch' && inputs.tag || github.ref_name }}",
		"push:",
		"tags:",
		`- "v*"`,
		"workflow_dispatch:",
		`ref: ${{ github.event_name == 'workflow_dispatch' && inputs.tag || github.ref }}`,
	} {
		if !strings.Contains(releaseWorkflow, expected) {
			t.Fatalf("release workflow does not contain %q", expected)
		}
	}
}

func TestMainHotfixDeliveryWorkflowUsesTrustedReleaseBoundary(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "hotfix-delivery.yml")
	for _, expected := range []string{
		"pull_request:",
		"- main",
		"- closed",
		"workflow_dispatch:",
		"startsWith(github.event.pull_request.head.ref, 'hotfix/')",
		"environment: release",
		"id-token: write",
		"google-github-actions/auth@7c6bc770dae815cd3e89ee6cdf493a5fab2cc093",
		"GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL",
		"GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN",
		"workflow hotfix verify-merge",
		"workflow hotfix verify-delivery",
		`git tag --annotate "$tag" "$merge_commit"`,
		`git push origin "refs/tags/$tag"`,
		"GITHUB_TOKEN: ${{ github.token }}",
		"actions/workflows/release.yml/dispatches",
		`\"inputs\":{\"tag\":\"${tag}\"}`,
		`rm -f "$response"`,
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("main hotfix delivery workflow does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"workflow hotfix propagate",
		"git cherry-pick",
		"--no-verify",
		"GIT_GOVERNANCE_GITHUB_TOKEN",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("main hotfix delivery workflow must not contain %q", forbidden)
		}
	}
}

func TestHotfixPropagationWorkflowUsesDedicatedPublisherBoundary(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "hotfix-propagation.yml")
	for _, expected := range []string{
		"workflow_dispatch:",
		"environment: release",
		"environment: hotfix-propagation",
		"needs: verify-delivery",
		"id-token: write",
		"test \"$GITHUB_REF\" = \"refs/heads/main\"",
		"GCP_BROKER_URL",
		"GCP_BROKER_WIF_PROVIDER",
		"GCP_BROKER_INVOKER_SERVICE_ACCOUNT",
		"GCP_HOTFIX_PROPAGATION_PUBLISHER_BROKER_URL",
		"GCP_HOTFIX_PROPAGATION_PUBLISHER_WIF_PROVIDER",
		"GCP_HOTFIX_PROPAGATION_PUBLISHER_INVOKER_SERVICE_ACCOUNT",
		"google-github-actions/auth@7c6bc770dae815cd3e89ee6cdf493a5fab2cc093",
		"GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL",
		"GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN",
		`GIT_GOVERNANCE_HOTFIX_PROPAGATION_PUBLISHER="server"`,
		`GIT_CONFIG_KEY_0="http.https://github.com/.extraheader"`,
		`echo "::add-mask::$token"`,
		"workflow hotfix verify-delivery",
		"workflow hotfix propagate-manifest",
		"--publish",
		`rm -f "$response"`,
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("hotfix propagation workflow does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"actions: write",
		"contents: write",
		"GITHUB_TOKEN",
		"git cherry-pick",
		"git push",
		"--no-verify",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("hotfix propagation workflow must not contain %q", forbidden)
		}
	}
}

func TestTagApprovalArtifactDispatchUsesJobToken(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "tag-approved-release.yml")
	dispatchStart := strings.Index(workflow, "- name: Dispatch artifact workflow for immutable tag")
	if dispatchStart == -1 {
		t.Fatal("tag workflow does not contain the artifact dispatch step")
	}

	dispatchStep := workflow[dispatchStart:]
	for _, expected := range []string{
		"GITHUB_TOKEN: ${{ github.token }}",
		`--header "Authorization: Bearer ${GITHUB_TOKEN}"`,
	} {
		if !strings.Contains(dispatchStep, expected) {
			t.Fatalf("artifact dispatch step does not contain %q", expected)
		}
	}
	if strings.Contains(dispatchStep, "GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}") {
		t.Fatal("artifact dispatch step must use the job token, not a repository secret")
	}
}

func TestCIWorkflowUsesBuildWorkspaceForNativeSmokeBinary(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "ci.yml")
	for _, expected := range []string{
		"mkdir -p .build/bin",
		`-o ".build/bin/git-governance${{ matrix.extension }}"`,
		`"./.build/bin/git-governance${{ matrix.extension }}" --version`,
		`"./.build/bin/git-governance${{ matrix.extension }}" --output json branch list`,
		`"./.build/bin/git-governance${{ matrix.extension }}" --output json policy describe`,
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("CI workflow does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"mkdir -p dist",
		`-o "dist/git-governance${{ matrix.extension }}"`,
		`"./dist/git-governance${{ matrix.extension }}"`,
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("CI workflow must not contain %q", forbidden)
		}
	}
}

func TestCIWorkflowValidatesMainHotfixRecordInsideRequiredQualityGate(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "ci.yml")
	for _, expected := range []string{
		"name: Validate reviewed main hotfix release record",
		"matrix.name == 'linux-amd64'",
		"github.event.pull_request.base.ref == 'main'",
		"startsWith(github.event.pull_request.head.ref, 'hotfix/')",
		"workflow hotfix validate-record",
		`--branch "$HOTFIX_BRANCH"`,
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("CI workflow does not validate main hotfix records with %q", expected)
		}
	}
}

func TestProtectedLineWorkflowKeepsSharedLineMutationInCI(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "create-protected-line.yml")
	for _, expected := range []string{
		"workflow_dispatch:",
		"run-name: Execute protected-line request ${{ inputs.request_id }}",
		"request_id:",
		"environment: release-execution",
		"name: Execute bound protected-line request",
		"workflow release execute-request",
		`git push origin "${SOURCE_SHA}:refs/heads/${TARGET}"`,
		"name: Finalize bound protected-line request",
		"workflow release finalize-request",
		"permissions:",
		"deployments: write",
		"actions: read",
		"GIT_GOVERNANCE_WORKFLOW_TOKEN: server",
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("protected-line workflow does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"inputs.kind",
		"inputs.version",
		"default: \"manual\"",
		"environment: release\n",
		"workflow release cut",
		"workflow release promote",
		"workflow release backmerge",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("protected-line workflow must not contain %q", forbidden)
		}
	}
}

func TestReleaseControlWorkflowUsesEphemeralBrokerIdentity(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "release-control.yml")
	for _, expected := range []string{
		"workflow_dispatch:",
		"broker-smoke",
		"release-request",
		"reconciliation-align",
		"reconciliation-resume",
		"kind:",
		"ticket_key:",
		"ticket:",
		"slug:",
		"resolution_branch:",
		"environment: release",
		"environment: release-request",
		"actions: write",
		"deployments: write",
		"id-token: write",
		"google-github-actions/auth@7c6bc770dae815cd3e89ee6cdf493a5fab2cc093",
		"token_format: id_token",
		"id_token_audience: ${{ vars.GCP_BROKER_URL }}",
		"GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL",
		"GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN",
		`"repository":"git-governance"`,
		`"repository":"not-approved"`,
		`test "$approved_status" = "200"`,
		`test "$rejected_status" = "403"`,
		`rm -f "$response"`,
		`test "$GITHUB_REF" = "refs/heads/main"`,
		"GITHUB_TOKEN: ${{ github.token }}",
		"GIT_GOVERNANCE_WORKFLOW_TOKEN: server",
		"workflow release request",
		`go build -mod=readonly -trimpath -o .build/bin/git-governance ./cmd/git-governance`,
		`workflow release stabilize`,
		`workflow release align-reconciliation-base`,
		`--prepared`,
		`git fetch origin "refs/heads/$RESOLUTION_BRANCH:refs/remotes/origin/$RESOLUTION_BRANCH"`,
		`git switch --create "$RESOLUTION_BRANCH" --track "origin/$RESOLUTION_BRANCH"`,
		`resolution_branch must match the supplied ticket and slug`,
		`git config --local http.https://github.com/.extraheader`,
		`git config --local --unset-all http.https://github.com/.extraheader`,
		`git config --local user.name "github-actions[bot]"`,
		`git config --local user.email "41898282+github-actions[bot]@users.noreply.github.com"`,
		`echo "::add-mask::$transport_token"`,
		`echo "::add-mask::$transport_header"`,
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("release-control workflow does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"GITHUB_RELEASE_APP_ID",
		"GITHUB_RELEASE_APP_INSTALLATION_ID",
		"echo \"$BROKER_ID_TOKEN\"",
		"echo \"$transport_token\"",
		"echo \"$transport_header\"",
		"cat \"$response\"",
		"workflow release cut \\\n            --version \"$VERSION\" \\\n            --dispatch",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("release-control workflow must not contain %q", forbidden)
		}
	}
}

func TestProtectedLineRecoveryWorkflowIsReadOnlyAndRequestBound(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "recover-protected-line-request.yml")
	for _, expected := range []string{
		"workflow_dispatch:",
		"request_id:",
		"refs/heads/main",
		"actions: read",
		"contents: read",
		"deployments: write",
		"GIT_GOVERNANCE_WORKFLOW_TOKEN: server",
		"workflow release finalize-request",
		"--recovery",
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("protected-line recovery workflow does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"environment:",
		"contents: write",
		"git push",
		"workflow release execute-request",
		"workflow release cut",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("protected-line recovery workflow must not contain %q", forbidden)
		}
	}
}

func TestReleaseControlWorkflowUsesDedicatedReconciliationPublisher(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "release-control.yml")
	jobStart := strings.Index(workflow, "  reconciliation-align:")
	if jobStart == -1 {
		t.Fatal("release-control workflow does not contain the reconciliation job")
	}
	job := workflow[jobStart:]

	for _, expected := range []string{
		"environment: release-reconciliation",
		"PUBLISHER_BROKER_URL: ${{ vars.GCP_RECONCILIATION_PUBLISHER_BROKER_URL }}",
		"PUBLISHER_WIF_PROVIDER: ${{ vars.GCP_RECONCILIATION_PUBLISHER_WIF_PROVIDER }}",
		"PUBLISHER_INVOKER_SERVICE_ACCOUNT: ${{ vars.GCP_RECONCILIATION_PUBLISHER_INVOKER_SERVICE_ACCOUNT }}",
		"workload_identity_provider: ${{ vars.GCP_RECONCILIATION_PUBLISHER_WIF_PROVIDER }}",
		"service_account: ${{ vars.GCP_RECONCILIATION_PUBLISHER_INVOKER_SERVICE_ACCOUNT }}",
		"id_token_audience: ${{ vars.GCP_RECONCILIATION_PUBLISHER_BROKER_URL }}",
		"id: publisher_auth",
		"PUBLISHER_BROKER_ID_TOKEN: ${{ steps.publisher_auth.outputs.id_token }}",
		`--request POST "$PUBLISHER_BROKER_URL/v1/github/installations/token"`,
		`--header "Authorization: Bearer $PUBLISHER_BROKER_ID_TOKEN"`,
		`export GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL="$PUBLISHER_BROKER_URL"`,
		`export GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN="$PUBLISHER_BROKER_ID_TOKEN"`,
	} {
		if !strings.Contains(job, expected) {
			t.Fatalf("reconciliation job does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"${{ vars.GCP_BROKER_URL }}",
		"${{ vars.GCP_BROKER_WIF_PROVIDER }}",
		"${{ vars.GCP_BROKER_INVOKER_SERVICE_ACCOUNT }}",
		"${{ steps.broker_auth.outputs.id_token }}",
	} {
		if strings.Contains(job, forbidden) {
			t.Fatalf("reconciliation job must not contain %q", forbidden)
		}
	}
}

func TestReleaseControlWorkflowConfiguresLocalReconciliationCommitIdentity(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "release-control.yml")
	stepStart := strings.Index(workflow, "- name: Create, align, or publish governed reconciliation branch")
	if stepStart == -1 {
		t.Fatal("release-control workflow does not contain the reconciliation step")
	}
	step := workflow[stepStart:]
	nameIndex := strings.Index(step, `git config --local user.name "github-actions[bot]"`)
	emailIndex := strings.Index(step, `git config --local user.email "41898282+github-actions[bot]@users.noreply.github.com"`)
	stabilizeIndex := strings.Index(step, `workflow release stabilize`)
	alignIndex := strings.Index(step, `workflow release align-reconciliation-base`)
	if nameIndex == -1 || emailIndex == -1 || stabilizeIndex == -1 || alignIndex == -1 ||
		nameIndex > stabilizeIndex || emailIndex > stabilizeIndex || nameIndex > alignIndex || emailIndex > alignIndex {
		t.Fatal("release-control workflow must configure the local commit identity before reconciliation commands")
	}
	for _, forbidden := range []string{
		"git config --global user.name",
		"git config --global user.email",
	} {
		if strings.Contains(step, forbidden) {
			t.Fatalf("release-control workflow must not contain %q", forbidden)
		}
	}
}

func readWorkflow(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(repositoryRoot(t), ".github", "workflows", name)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
