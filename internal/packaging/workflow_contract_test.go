package packaging

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

var lifecyclePayloads = []string{
	"reusable-release-control.yml",
	"reusable-execute-protected-line-request.yml",
	"reusable-release-reconciliation.yml",
	"reusable-tag-promoted-release.yml",
	"reusable-publish-release-artifacts.yml",
	"reusable-hotfix-delivery.yml",
	"reusable-hotfix-propagation.yml",
}

var lifecycleCallers = []string{
	"release-control.yml",
	"release-reconciliation.yml",
	"execute-protected-line-request.yml",
	"tag-promoted-release.yml",
	"publish-release-artifacts.yml",
	"hotfix-delivery.yml",
	"hotfix-propagation.yml",
}

// eventDrivenEnvironmentGatedCallers are the callers whose event-driven
// detection job dispatches the environment-gated execution on the default
// branch; only they may carry the bounded detection steps.
var eventDrivenEnvironmentGatedCallers = map[string]bool{
	"tag-promoted-release.yml": true,
	"hotfix-delivery.yml":      true,
}

var actionReferencePattern = regexp.MustCompile(`^[a-z0-9_.-]+/[a-z0-9_.-]+(/[a-z0-9_.-]+)*@[0-9a-f]{40} # .+$`)

func TestLifecyclePayloadsCarryOnlyWorkflowCall(t *testing.T) {
	t.Parallel()

	for _, name := range lifecyclePayloads {
		workflow := readWorkflow(t, name)
		if !strings.Contains(workflow, "on:\n  workflow_call:") {
			t.Fatalf("%s does not carry the workflow_call trigger", name)
		}
		for _, forbidden := range []string{
			"workflow_dispatch:",
			"pull_request:",
			"schedule:",
			"repository_dispatch",
			"\n  push:",
		} {
			if strings.Contains(workflow, forbidden) {
				t.Fatalf("%s must not self-trigger with %q", name, forbidden)
			}
		}
	}
}

func TestLifecyclePayloadsPinEveryActionReference(t *testing.T) {
	t.Parallel()

	for _, name := range lifecyclePayloads {
		workflow := readWorkflow(t, name)
		for line := range strings.Lines(workflow) {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "uses: ") {
				continue
			}
			reference := strings.TrimSpace(strings.TrimPrefix(trimmed, "uses: "))
			if !actionReferencePattern.MatchString(reference) {
				t.Fatalf("%s carries the unpinned action reference %q", name, reference)
			}
		}
	}
}

func TestLifecyclePayloadsCarryNoOrganizationOrCredentialLiterals(t *testing.T) {
	t.Parallel()

	for _, name := range lifecyclePayloads {
		workflow := readWorkflow(t, name)
		for _, forbidden := range []string{
			"t33n-software",
			"CyberT33N",
			"git-governance-release-broker",
			"git-governance-broker-instance",
			"t33n-software-platform-instance",
			"pkg.dev",
			"europe-west3",
			"ghp_",
			"gho_",
			"PRIVATE KEY",
			"GIT_GOVERNANCE_GITHUB_APP_CLIENT_ID",
		} {
			if strings.Contains(workflow, forbidden) {
				t.Fatalf("%s must not contain %q", name, forbidden)
			}
		}
	}
}

func TestLifecyclePayloadsBindEnvironmentsAsInputs(t *testing.T) {
	t.Parallel()

	for _, name := range lifecyclePayloads {
		workflow := readWorkflow(t, name)
		if !strings.Contains(workflow, "environment: ${{ inputs.") {
			t.Fatalf("%s does not bind its environment as a typed input", name)
		}
		for _, forbidden := range []string{
			"environment: release\n",
			"environment: release-request\n",
			"environment: release-execution\n",
			"environment: release-delivery\n",
			"environment: release-reconciliation\n",
			"environment: release-credential-verification\n",
			"environment: hotfix-delivery\n",
			"environment: hotfix-propagation\n",
		} {
			if strings.Contains(workflow, forbidden) {
				t.Fatalf("%s must not hard-code the lane environment %q", name, forbidden)
			}
		}
	}
}

func TestLifecyclePayloadsBindTheDeliveryVariant(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"reusable-release-control.yml",
		"reusable-release-reconciliation.yml",
		"reusable-publish-release-artifacts.yml",
		"reusable-hotfix-delivery.yml",
		"reusable-hotfix-propagation.yml",
	} {
		workflow := readWorkflow(t, name)
		for _, expected := range []string{
			"delivery_variant:",
			"type: string",
			"(one of cloud or github-only)",
		} {
			if !strings.Contains(workflow, expected) {
				t.Fatalf("%s does not bind the delivery variant with %q", name, expected)
			}
		}
	}

	for _, name := range []string{
		"reusable-hotfix-delivery.yml",
		"reusable-hotfix-propagation.yml",
	} {
		workflow := readWorkflow(t, name)
		if !strings.Contains(workflow, "the github-only delivery variant of this lane is bound with the lane evidence-path migration") {
			t.Fatalf("%s does not carry the named github-only deferral", name)
		}
	}

	publish := readWorkflow(t, "reusable-publish-release-artifacts.yml")
	for _, expected := range []string{
		"name: Deliver release artifacts (cloud)",
		"name: Deliver release artifacts (github-only)",
		"if: ${{ inputs.delivery_variant == 'cloud' }}",
		"if: ${{ inputs.delivery_variant == 'github-only' }}",
	} {
		if !strings.Contains(publish, expected) {
			t.Fatalf("publish payload does not contain %q", expected)
		}
	}
}

func TestLifecycleWorkflowFilesAreWellFormedYAML(t *testing.T) {
	t.Parallel()

	for _, name := range lifecyclePayloads {
		var document any
		if err := yaml.Unmarshal([]byte(readWorkflow(t, name)), &document); err != nil {
			t.Fatalf("payload %s is not well-formed YAML: %v", name, err)
		}
	}
	for _, name := range lifecycleCallers {
		var document any
		if err := yaml.Unmarshal([]byte(readLifecycleCallerMaster(t, name)), &document); err != nil {
			t.Fatalf("caller master %s is not well-formed YAML: %v", name, err)
		}
	}
}

func TestLifecyclePayloadsUseOnlyWorkflowCallInputTypes(t *testing.T) {
	t.Parallel()

	for _, name := range lifecyclePayloads {
		workflow := readWorkflow(t, name)
		for _, forbidden := range []string{
			"type: choice",
			"options:",
		} {
			if strings.Contains(workflow, forbidden) {
				t.Fatalf("%s carries %q, which workflow_call inputs do not support (only string, number, and boolean are valid)", name, forbidden)
			}
		}
	}
}

func TestLifecyclePayloadsProvisionTheExactPinnedToolchain(t *testing.T) {
	t.Parallel()

	expectedSetupCounts := map[string]int{
		"reusable-release-control.yml":                1,
		"reusable-execute-protected-line-request.yml": 3,
		"reusable-release-reconciliation.yml":         1,
		"reusable-tag-promoted-release.yml":           0,
		"reusable-publish-release-artifacts.yml":      2,
		"reusable-hotfix-delivery.yml":                1,
		"reusable-hotfix-propagation.yml":             2,
	}
	if len(expectedSetupCounts) != len(lifecyclePayloads) {
		t.Fatal("every lifecycle payload must declare its expected toolchain setup count")
	}

	for _, name := range lifecyclePayloads {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			workflow := readWorkflow(t, name)
			wantSetups, found := expectedSetupCounts[name]
			if !found {
				t.Fatalf("%s is missing from the expected setup-count contract", name)
			}
			if got := strings.Count(workflow, "uses: actions/setup-go@"); got != wantSetups {
				t.Fatalf("%s carries %d setup-go invocations, want %d", name, got, wantSetups)
			}
			if strings.Contains(workflow, "go-version-file:") {
				t.Fatalf("%s must not resolve Go from the go directive through go-version-file", name)
			}
			if got := strings.Count(workflow, "- name: Resolve the pinned toolchain"); got != wantSetups {
				t.Fatalf("%s carries %d pinned-toolchain resolvers, want %d", name, got, wantSetups)
			}
			if got := strings.Count(workflow, `go-version: ${{ steps.toolchain.outputs.version }}`); got != wantSetups {
				t.Fatalf("%s carries %d exact setup-go versions, want %d", name, got, wantSetups)
			}
			if wantSetups == 0 {
				return
			}

			for _, required := range []string{
				`awk '{ sub(/\r$/, "") }`,
				`$1 == "toolchain" { count += 1; directive = $2; trailing = $3 }`,
				`if (count != 1) exit 1`,
				`if (trailing != "" && trailing !~ /^\/\//) exit 1`,
				`if (directive !~ /^go[0-9]+\.[0-9]+\.[0-9]+$/) exit 1`,
				`echo "version=${directive#go}" >> "$GITHUB_OUTPUT"`,
			} {
				if !strings.Contains(workflow, required) {
					t.Fatalf("%s does not validate the pinned toolchain directive with %q", name, required)
				}
			}
		})
	}
}

func TestLifecyclePayloadsBindTheGovernanceCLIWithoutMigrationSeams(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"reusable-release-control.yml",
		"reusable-execute-protected-line-request.yml",
		"reusable-release-reconciliation.yml",
		"reusable-hotfix-delivery.yml",
		"reusable-hotfix-propagation.yml",
	} {
		workflow := readWorkflow(t, name)
		for _, expected := range []string{
			"name: Bind the governance CLI",
			`"github.com/${GOVERNANCE_REPOSITORY}/cmd/git-governance"`,
			"go build -mod=readonly -trimpath",
			"go build -modfile tools/go.mod -trimpath",
		} {
			if !strings.Contains(workflow, expected) {
				t.Fatalf("%s does not bind the governance CLI with %q", name, expected)
			}
		}
		for _, forbidden := range []string{
			"Build trusted governance CLI",
			"Check out the pinned governance",
			"governance_ref",
			"GOVERNANCE_REF",
			"GIT_GOVERNANCE_SOURCE_REF",
		} {
			if strings.Contains(workflow, forbidden) {
				t.Fatalf("%s must not carry the removed migration seam %q", name, forbidden)
			}
		}
	}

	for _, name := range lifecyclePayloads {
		workflow := readWorkflow(t, name)
		for _, forbidden := range []string{
			"source_gate",
			"SOURCE_GATE",
			"GIT_GOVERNANCE_SOURCE_GATE",
		} {
			if strings.Contains(workflow, forbidden) {
				t.Fatalf("%s must not carry the removed source-gate seam %q", name, forbidden)
			}
		}
	}
}

func TestTagPromotionPayloadDispatchesReleaseArtifacts(t *testing.T) {
	t.Parallel()

	tagPayload := readWorkflow(t, "reusable-tag-promoted-release.yml")
	for _, expected := range []string{
		"name: Reusable Tag Promoted Release",
		"release_branch:",
		"merge_sha:",
		"delivery_environment:",
		`git tag --annotate "${TAG}" "${MERGE_SHA}" --message "Release ${VERSION}"`,
		`git push origin "refs/tags/${TAG}"`,
		"actions/workflows/publish-release-artifacts.yml/dispatches",
		`\"inputs\":{\"tag\":\"${TAG}\"}`,
	} {
		if !strings.Contains(tagPayload, expected) {
			t.Fatalf("tag payload does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"repository_dispatch",
		"github.event.pull_request",
	} {
		if strings.Contains(tagPayload, forbidden) {
			t.Fatalf("tag payload must not contain %q", forbidden)
		}
	}

	releasePayload := readWorkflow(t, "reusable-publish-release-artifacts.yml")
	for _, expected := range []string{
		"name: Reusable Publish Release Artifacts",
		"tag:",
		"go tool -modfile tools/go.mod quality-gate",
		"environment: ${{ inputs.delivery_environment }}",
	} {
		if !strings.Contains(releasePayload, expected) {
			t.Fatalf("release payload does not contain %q", expected)
		}
	}
}

func TestPublishPayloadProvisionsTheSignerViaTheGovernedChannel(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "reusable-publish-release-artifacts.yml")
	for _, expected := range []string{
		"name: Provision the bound signature verifier",
		`verifier="$(go tool -modfile tools/go.mod quality-gate provision-verifier)"`,
		`dirname "$verifier" >> "$GITHUB_PATH"`,
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("publish payload does not provision the signer via the governed channel with %q", expected)
		}
	}
	for _, forbidden := range []string{
		"sigstore/cosign-installer",
		"cosign-release:",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("publish payload must not carry the payload-level installer form %q: the signer identity is defined once in the registry and provisioned via the governed machinery", forbidden)
		}
	}

	provisionIndex := strings.Index(workflow, "quality-gate provision-verifier")
	signIndex := strings.Index(workflow, "name: Build, package, sign, and publish")
	if provisionIndex == -1 || signIndex == -1 || provisionIndex > signIndex {
		t.Fatal("publish payload must provision the signer before the signing release step")
	}

	githubOnlyStart := strings.Index(workflow, "  deliver-github:")
	if githubOnlyStart == -1 {
		t.Fatal("publish payload does not contain the github-only delivery job")
	}
	if strings.Contains(workflow[githubOnlyStart:], "provision-verifier") {
		t.Fatal("the github-only delivery variant does not sign with cosign and must not provision the verifier")
	}
}

func TestTagPromotionArtifactDispatchUsesJobToken(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "reusable-tag-promoted-release.yml")
	dispatchStart := strings.Index(workflow, "- name: Dispatch artifact workflow for immutable tag")
	if dispatchStart == -1 {
		t.Fatal("tag payload does not contain the artifact dispatch step")
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

func TestMainHotfixDeliveryPayloadUsesTrustedHotfixDeliveryBoundary(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "reusable-hotfix-delivery.yml")
	for _, expected := range []string{
		"environment: ${{ inputs.delivery_environment }}",
		"id-token: write",
		"google-github-actions/auth@7c6bc770dae815cd3e89ee6cdf493a5fab2cc093",
		"GCP_HOTFIX_DELIVERY_BROKER_URL",
		"GCP_HOTFIX_DELIVERY_WIF_PROVIDER",
		"GCP_HOTFIX_DELIVERY_INVOKER_SERVICE_ACCOUNT",
		"GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL",
		"GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN",
		"workflow hotfix verify-merge",
		"workflow hotfix verify-delivery",
		`git tag --annotate "$tag" "$merge_commit"`,
		`git push origin "refs/tags/$tag"`,
		"GITHUB_TOKEN: ${{ github.token }}",
		"actions/workflows/publish-release-artifacts.yml/dispatches",
		`\"inputs\":{\"tag\":\"${tag}\"}`,
		`rm -f "$response"`,
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("main hotfix delivery payload does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"workflow hotfix propagate",
		"git cherry-pick",
		"--no-verify",
		"GIT_GOVERNANCE_GITHUB_TOKEN",
		"GCP_BROKER_URL",
		"GCP_BROKER_WIF_PROVIDER",
		"GCP_BROKER_INVOKER_SERVICE_ACCOUNT",
		"github.event.pull_request",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("main hotfix delivery payload must not contain %q", forbidden)
		}
	}
}

func TestHotfixPropagationPayloadUsesDedicatedPublisherBoundary(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "reusable-hotfix-propagation.yml")
	for _, expected := range []string{
		"environment: ${{ inputs.delivery_environment }}",
		"environment: ${{ inputs.propagation_environment }}",
		"needs: verify-delivery",
		"id-token: write",
		"test \"$GITHUB_REF\" = \"refs/heads/main\"",
		"GCP_HOTFIX_DELIVERY_BROKER_URL",
		"GCP_HOTFIX_DELIVERY_WIF_PROVIDER",
		"GCP_HOTFIX_DELIVERY_INVOKER_SERVICE_ACCOUNT",
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
			t.Fatalf("hotfix propagation payload does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"actions: write",
		"contents: write",
		"GITHUB_TOKEN",
		"git cherry-pick",
		"git push",
		"--no-verify",
		"GCP_BROKER_URL",
		"GCP_BROKER_WIF_PROVIDER",
		"GCP_BROKER_INVOKER_SERVICE_ACCOUNT",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("hotfix propagation payload must not contain %q", forbidden)
		}
	}
}

func TestProtectedLineExecutorPayloadKeepsSharedLineMutationInCI(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "reusable-execute-protected-line-request.yml")
	for _, expected := range []string{
		"request_id:",
		"recovery:",
		"type: boolean",
		"repository:",
		"execution_environment:",
		"environment: ${{ inputs.execution_environment }}",
		"name: Execute bound protected-line request",
		"workflow release execute-request",
		`git push origin "${SOURCE_SHA}:refs/heads/${TARGET}"`,
		"name: Finalize bound protected-line request",
		"workflow release finalize-request",
		"permissions:",
		"deployments: write",
		"actions: read",
		"GIT_GOVERNANCE_WORKFLOW_TOKEN: server",
		"the protected-line executor is bound to",
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("protected-line executor payload does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"inputs.kind",
		"inputs.version",
		`default: "manual"`,
		"environment: release\n",
		"workflow release cut",
		"workflow release promote",
		"workflow release backmerge",
		"github.repository == '",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("protected-line executor payload must not contain %q", forbidden)
		}
	}
}

func TestProtectedLineRecoveryIsABoundExecutorMode(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "reusable-execute-protected-line-request.yml")
	for _, expected := range []string{
		"name: Recover bound protected-line request",
		"if: ${{ inputs.recovery == true }}",
		"refs/heads/main",
		"workflow release finalize-request",
		"--recovery",
		"protected-line recovery runs only from the main line",
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("protected-line executor payload recovery mode does not contain %q", expected)
		}
	}

	recoverStart := strings.Index(workflow, "  recover:")
	if recoverStart == -1 {
		t.Fatal("protected-line executor payload does not contain the recover job")
	}
	recoverJob := workflow[recoverStart:]
	for _, forbidden := range []string{
		"contents: write",
		"git push",
		"workflow release execute-request",
		"environment:",
	} {
		if strings.Contains(recoverJob, forbidden) {
			t.Fatalf("protected-line recovery mode must not contain %q", forbidden)
		}
	}

	legacyPath := filepath.Join(repositoryRoot(t), ".github", "workflows", "recover-protected-line-request.yml")
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("the separate recovery workflow must be folded into the executor payload, err=%v", err)
	}
}

func TestReleaseControlPayloadUsesEphemeralBrokerIdentity(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "reusable-release-control.yml")
	for _, expected := range []string{
		"broker-smoke",
		"release-request",
		"kind:",
		"ticket_key:",
		"ticket:",
		"environment: ${{ inputs.verification_environment }}",
		"environment: ${{ inputs.request_environment }}",
		"actions: write",
		"deployments: write",
		"id-token: write",
		"google-github-actions/auth@7c6bc770dae815cd3e89ee6cdf493a5fab2cc093",
		"token_format: id_token",
		"GCP_RELEASE_CREDENTIAL_VERIFICATION_BROKER_URL",
		"GCP_RELEASE_CREDENTIAL_VERIFICATION_WIF_PROVIDER",
		"GCP_RELEASE_CREDENTIAL_VERIFICATION_INVOKER_SERVICE_ACCOUNT",
		"id_token_audience: ${{ vars.GCP_RELEASE_CREDENTIAL_VERIFICATION_BROKER_URL }}",
		`"repository\":\"not-approved\"`,
		`test "$approved_status" = "200"`,
		`test "$rejected_status" = "403"`,
		`rm -f "$response"`,
		`test "$GITHUB_REF" = "refs/heads/main"`,
		"GITHUB_TOKEN: ${{ github.token }}",
		"GIT_GOVERNANCE_WORKFLOW_TOKEN: server",
		"workflow release request",
		"broker-smoke verifies the cloud credential boundary and is not bound in the github-only delivery variant",
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("release-control payload does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"GITHUB_RELEASE_APP_ID",
		"GITHUB_RELEASE_APP_INSTALLATION_ID",
		"echo \"$BROKER_ID_TOKEN\"",
		"cat \"$response\"",
		"environment: release\n",
		"GCP_BROKER_URL",
		"GCP_BROKER_WIF_PROVIDER",
		"GCP_BROKER_INVOKER_SERVICE_ACCOUNT",
		"reconciliation-align",
		"reconciliation-resume",
		"workflow release stabilize",
		"workflow release align-reconciliation-base",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("release-control payload must not contain %q", forbidden)
		}
	}
}

func TestReleaseReconciliationPayloadUsesDedicatedPublisherBoundary(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "reusable-release-reconciliation.yml")
	for _, expected := range []string{
		"reconciliation-align",
		"reconciliation-resume",
		"resolution_branch:",
		"environment: ${{ inputs.reconciliation_environment }}",
		"PUBLISHER_BROKER_URL: ${{ vars.GCP_RECONCILIATION_PUBLISHER_BROKER_URL }}",
		"PUBLISHER_WIF_PROVIDER: ${{ vars.GCP_RECONCILIATION_PUBLISHER_WIF_PROVIDER }}",
		"PUBLISHER_INVOKER_SERVICE_ACCOUNT: ${{ vars.GCP_RECONCILIATION_PUBLISHER_INVOKER_SERVICE_ACCOUNT }}",
		"workload_identity_provider: ${{ vars.GCP_RECONCILIATION_PUBLISHER_WIF_PROVIDER }}",
		"service_account: ${{ vars.GCP_RECONCILIATION_PUBLISHER_INVOKER_SERVICE_ACCOUNT }}",
		"id_token_audience: ${{ vars.GCP_RECONCILIATION_PUBLISHER_BROKER_URL }}",
		"id: publisher_auth",
		"PUBLISHER_BROKER_ID_TOKEN: ${{ steps.publisher_auth.outputs.id_token || '' }}",
		`--request POST "$PUBLISHER_BROKER_URL/v1/github/installations/token"`,
		`--header "Authorization: Bearer $PUBLISHER_BROKER_ID_TOKEN"`,
		`export GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL="$PUBLISHER_BROKER_URL"`,
		`export GIT_GOVERNANCE_WORKLOAD_IDENTITY_TOKEN="$PUBLISHER_BROKER_ID_TOKEN"`,
		"workflow release stabilize",
		"workflow release align-reconciliation-base",
		"--prepared",
		`git fetch origin "refs/heads/$RESOLUTION_BRANCH:refs/remotes/origin/$RESOLUTION_BRANCH"`,
		`git switch --create "$RESOLUTION_BRANCH" --track "origin/$RESOLUTION_BRANCH"`,
		"resolution_branch must match the supplied ticket and slug",
		`git config --local http.https://github.com/.extraheader`,
		`git config --local --unset-all http.https://github.com/.extraheader`,
		"persist-credentials: ${{ inputs.delivery_variant == 'github-only' }}",
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("release-reconciliation payload does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"${{ vars.GCP_BROKER_URL }}",
		"${{ vars.GCP_BROKER_WIF_PROVIDER }}",
		"${{ vars.GCP_BROKER_INVOKER_SERVICE_ACCOUNT }}",
		"${{ steps.broker_auth.outputs.id_token }}",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("release-reconciliation payload must not contain %q", forbidden)
		}
	}
}

func TestReleaseReconciliationPayloadConfiguresLocalCommitIdentity(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "reusable-release-reconciliation.yml")
	stepStart := strings.Index(workflow, "- name: Create, align, or publish governed reconciliation branch")
	if stepStart == -1 {
		t.Fatal("release-reconciliation payload does not contain the reconciliation step")
	}
	step := workflow[stepStart:]
	nameIndex := strings.Index(step, `git config --local user.name "github-actions[bot]"`)
	emailIndex := strings.Index(step, `git config --local user.email "41898282+github-actions[bot]@users.noreply.github.com"`)
	stabilizeIndex := strings.Index(step, `workflow release stabilize`)
	alignIndex := strings.Index(step, `workflow release align-reconciliation-base`)
	if nameIndex == -1 || emailIndex == -1 || stabilizeIndex == -1 || alignIndex == -1 ||
		nameIndex > stabilizeIndex || emailIndex > stabilizeIndex || nameIndex > alignIndex || emailIndex > alignIndex {
		t.Fatal("release-reconciliation payload must configure the local commit identity before reconciliation commands")
	}
	for _, forbidden := range []string{
		"git config --global user.name",
		"git config --global user.email",
	} {
		if strings.Contains(step, forbidden) {
			t.Fatalf("release-reconciliation payload must not contain %q", forbidden)
		}
	}
}

func TestLifecycleCallersAreByteIdenticalToTheMasters(t *testing.T) {
	t.Parallel()

	for _, name := range lifecycleCallers {
		caller := readWorkflow(t, name)
		master := readLifecycleCallerMaster(t, name)
		if caller != master {
			t.Fatalf("caller %s is not byte-identical to its canonical master", name)
		}
	}
}

func TestLifecycleCallersCarryTheContractSurface(t *testing.T) {
	t.Parallel()

	for _, name := range lifecycleCallers {
		caller := readWorkflow(t, name)
		payloadName := "reusable-" + name
		for _, expected := range []string{
			"permissions: {}",
			"uses: t33n-software/git-governance/.github/workflows/" + payloadName + "@",
			"name: ",
			"permissions:",
		} {
			if !strings.Contains(caller, expected) {
				t.Fatalf("caller %s does not contain %q", name, expected)
			}
		}
		if strings.Contains(caller, "steps:") && !eventDrivenEnvironmentGatedCallers[name] {
			t.Fatalf("caller %s must stay a thin caller without steps", name)
		}
		for _, forbidden := range []string{
			"uses: actions/",
			"uses: google-github-actions/",
			"uses: goreleaser/",
			"uses: sigstore/",
			"uses: anchore/",
		} {
			if strings.Contains(caller, forbidden) {
				t.Fatalf("caller %s must not carry the action reference %q", name, forbidden)
			}
		}
	}
}

func TestEventDrivenEnvironmentGatedCallersExecuteOnTheMainBoundRun(t *testing.T) {
	t.Parallel()

	for name, dispatchPath := range map[string]string{
		"tag-promoted-release.yml": "actions/workflows/tag-promoted-release.yml/dispatches",
		"hotfix-delivery.yml":      "actions/workflows/hotfix-delivery.yml/dispatches",
	} {
		caller := readWorkflow(t, name)
		for _, expected := range []string{
			"pull_request:",
			"workflow_dispatch:",
			"  detect:",
			"if: github.event_name == 'workflow_dispatch'",
			`\"ref\":\"main\"`,
			dispatchPath,
		} {
			if !strings.Contains(caller, expected) {
				t.Fatalf("event-driven environment-gated caller %s does not contain %q", name, expected)
			}
		}

		detectStart := strings.Index(caller, "  detect:")
		executeStart := strings.Index(caller, "if: github.event_name == 'workflow_dispatch'")
		if detectStart == -1 || executeStart == -1 || executeStart < detectStart {
			t.Fatalf("caller %s does not order the detection job before the main-bound execution job", name)
		}
		detectJob := caller[detectStart:executeStart]
		if !strings.Contains(detectJob, "actions: write") {
			t.Fatalf("caller %s detection job does not carry the dispatch grant", name)
		}
		for _, forbidden := range []string{
			"environment:",
			"contents: write",
			"id-token: write",
			"uses:",
		} {
			if strings.Contains(detectJob, forbidden) {
				t.Fatalf("caller %s detection job must not contain %q: the environment-gated call never runs on a pull_request run", name, forbidden)
			}
		}
	}
}

func TestLifecycleCallersPinTheBoundPayloadCommit(t *testing.T) {
	t.Parallel()

	record := readLifecycleCallerHashRecord(t)
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(record.Home.SHA) {
		t.Fatalf("the hash record home SHA %q is not a full-length commit SHA", record.Home.SHA)
	}
	for _, name := range lifecycleCallers {
		caller := readWorkflow(t, name)
		payloadName := "reusable-" + name
		if !strings.Contains(caller, payloadName+"@"+record.Home.SHA) {
			t.Fatalf("caller %s does not pin the payload at the recorded home SHA", name)
		}
	}
}

func TestLifecycleCallerPinIsOnTheMergedDevelopLine(t *testing.T) {
	record := readLifecycleCallerHashRecord(t)
	cmd := exec.Command("git", "merge-base", "--is-ancestor", record.Home.SHA, "origin/develop")
	cmd.Dir = repositoryRoot(t)
	if err := cmd.Run(); err != nil {
		t.Fatalf("the pinned home SHA %q is not reachable from origin/develop; the payload pin must bind a commit on the merged develop line, never a pull-request-only commit that the cross-repo resolver cannot find", record.Home.SHA)
	}
}

func TestLifecycleCallerHashRecordIsConsistent(t *testing.T) {
	t.Parallel()

	record := readLifecycleCallerHashRecord(t)
	if record.Home.Repository != "t33n-software/git-governance" {
		t.Fatalf("the hash record binds the wrong home %q", record.Home.Repository)
	}
	if len(record.Callers) != len(lifecycleCallers) {
		t.Fatalf("the hash record carries %d callers, want %d", len(record.Callers), len(lifecycleCallers))
	}
	for _, entry := range record.Callers {
		master := readLifecycleCallerMaster(t, filepath.Base(entry.Master))
		sum := sha256.Sum256([]byte(master))
		if fmt.Sprintf("%x", sum) != entry.SHA256 {
			t.Fatalf("the recorded hash of %s does not match its content", entry.Master)
		}
		expectedMaster := "workflows/github/callers/release-lifecycle/" + filepath.Base(entry.Master)
		if entry.Master != expectedMaster {
			t.Fatalf("the hash record master path %q is not the canonical %q", entry.Master, expectedMaster)
		}
		expectedTenant := ".github/workflows/" + filepath.Base(entry.Master)
		if entry.TenantFile != expectedTenant {
			t.Fatalf("the hash record tenant file %q is not the canonical %q", entry.TenantFile, expectedTenant)
		}
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		input    string
		expected string
	}{
		{name: "crlf folds to lf", input: "a\r\nb\r\n", expected: "a\nb\n"},
		{name: "lf unchanged", input: "a\nb\n", expected: "a\nb\n"},
		{name: "empty", input: "", expected: ""},
		{name: "lone carriage return preserved", input: "a\rb\n", expected: "a\rb\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeLineEndings(testCase.input); got != testCase.expected {
				t.Fatalf("normalizeLineEndings(%q) = %q, want %q", testCase.input, got, testCase.expected)
			}
		})
	}
}

func TestLifecycleCallerHashProofIsLineEndingStable(t *testing.T) {
	t.Parallel()

	record := readLifecycleCallerHashRecord(t)
	for _, entry := range record.Callers {
		master := readLifecycleCallerMaster(t, filepath.Base(entry.Master))
		crlfCheckout := strings.ReplaceAll(master, "\n", "\r\n")
		sum := sha256.Sum256([]byte(normalizeLineEndings(crlfCheckout)))
		if fmt.Sprintf("%x", sum) != entry.SHA256 {
			t.Fatalf("the hash proof of %s is not stable across checkout line endings", entry.Master)
		}
	}
}

func TestWorkflowFilesUseBoundedLifecycleNames(t *testing.T) {
	t.Parallel()

	for name, expected := range map[string]string{
		"execute-protected-line-request.yml": "name: Execute Protected-Line Request",
		"tag-promoted-release.yml":           "name: Tag Promoted Release",
		"publish-release-artifacts.yml":      "name: Publish Release Artifacts",
		"release-reconciliation.yml":         "name: Governed Release Reconciliation",
	} {
		if !strings.Contains(readWorkflow(t, name), expected) {
			t.Fatalf("%s does not contain %q", name, expected)
		}
	}

	for _, legacyName := range []string{
		"create-protected-line.yml",
		"tag-approved-release.yml",
		"release.yml",
		"recover-protected-line-request.yml",
	} {
		path := filepath.Join(repositoryRoot(t), ".github", "workflows", legacyName)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy workflow path %s must be absent, err=%v", legacyName, err)
		}
	}
}

func TestReleaseGatesWorkflowValidatesMainHotfixRecord(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "release-gates.yml")
	for _, expected := range []string{
		"  hotfix-record:\n",
		"name: Hotfix release record",
		"github.event_name == 'pull_request' &&",
		"github.event.pull_request.base.ref == 'main' &&",
		"github.event.pull_request.head.repo.full_name == github.repository &&",
		"startsWith(github.event.pull_request.head.ref, 'hotfix/')",
		"name: Validate reviewed main hotfix release record",
		"workflow hotfix validate-record",
		`--branch "$HOTFIX_BRANCH"`,
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("release-gates workflow does not validate main hotfix records with %q", expected)
		}
	}
}

func TestReleaseGatesWorkflowValidatesTheReleaseConfiguration(t *testing.T) {
	t.Parallel()

	workflow := readWorkflow(t, "release-gates.yml")
	for _, expected := range []string{
		"  release-config:\n",
		"name: Release configuration",
		"goreleaser/goreleaser-action@5daf1e915a5f0af01ddbcd89a43b8061ff4f1a89",
		"args: check",
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("release-gates workflow does not validate the release configuration with %q", expected)
		}
	}
}

// bindingManifest mirrors the tenant binding manifest (repo-bindings/v1) for
// the self-consistency proofs of the canonical adoption. The home-side proof
// against the canonical masters is owned by the verify-canonical tool; these
// tests bind the tenant files to the manifest.
type bindingManifest struct {
	Home struct {
		Repository string `json:"repository"`
		SHA        string `json:"sha"`
	} `json:"home"`
	Callers []struct {
		File   string `json:"file"`
		Master string `json:"master"`
		SHA256 string `json:"sha256"`
	} `json:"callers"`
	Files struct {
		Lefthook      fileBinding `json:"lefthook"`
		Gitattributes fileBinding `json:"gitattributes"`
		Gitignore     fileBinding `json:"gitignore"`
		Dependabot    fileBinding `json:"dependabot"`
	} `json:"files"`
	Codeowners struct {
		Path         string `json:"path"`
		DefaultOwner string `json:"defaultOwner"`
	} `json:"codeowners"`
}

type fileBinding struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func readBindingManifest(t *testing.T) bindingManifest {
	t.Helper()

	var manifest bindingManifest
	if err := json.Unmarshal([]byte(readRepositoryDocument(t, "repo-bindings.json")), &manifest); err != nil {
		t.Fatalf("repo-bindings.json is not valid JSON: %v", err)
	}
	if manifest.Home.Repository != "t33n-software/repository-governance" {
		t.Fatalf("the manifest binds home %q", manifest.Home.Repository)
	}
	return manifest
}

// hashRepositoryFile hashes the LF-normalized repository file; the canonical
// .gitattributes makes the checkout LF, and the normalization keeps the
// derivation tolerant as the second line of defense.
func hashRepositoryFile(t *testing.T, path string) string {
	t.Helper()

	sum := sha256.Sum256([]byte(normalizeLineEndings(readRepositoryDocument(t, path))))
	return fmt.Sprintf("%x", sum)
}

func TestCanonicalCallersMatchTheBindingManifest(t *testing.T) {
	t.Parallel()

	manifest := readBindingManifest(t)
	want := map[string]string{
		".github/workflows/ci.yml":                "hosting-platforms/github/workflows/callers/go/ci-full.yml",
		".github/workflows/codeql.yml":            "hosting-platforms/github/workflows/callers/go/codeql.yml",
		".github/workflows/dependency-review.yml": "hosting-platforms/github/workflows/callers/go/dependency-review.yml",
	}
	if len(manifest.Callers) != len(want) {
		t.Fatalf("the manifest carries %d callers, want %d", len(manifest.Callers), len(want))
	}
	for _, caller := range manifest.Callers {
		master, found := want[caller.File]
		if !found {
			t.Fatalf("the manifest carries an unexpected caller %q", caller.File)
		}
		if caller.Master != master {
			t.Fatalf("caller %q binds master %q, want %q", caller.File, caller.Master, master)
		}
		if hash := hashRepositoryFile(t, caller.File); hash != caller.SHA256 {
			t.Fatalf("the tenant caller %s hashes to %s, want the bound %s", caller.File, hash, caller.SHA256)
		}
		content := readRepositoryDocument(t, caller.File)
		if !strings.Contains(content, "uses: "+manifest.Home.Repository+"/.github/workflows/reusable-") {
			t.Fatalf("the tenant caller %s does not reference a home payload", caller.File)
		}
		if !strings.Contains(content, "@"+manifest.Home.SHA) {
			t.Fatalf("the tenant caller %s does not pin the bound home SHA", caller.File)
		}
		if !strings.Contains(content, `branches: [main, develop, "release/**", "support/**"]`) {
			t.Fatalf("the tenant caller %s does not cover every shared line", caller.File)
		}
	}
}

func TestCanonicalFileFamilyMatchesTheBindingManifest(t *testing.T) {
	t.Parallel()

	manifest := readBindingManifest(t)
	for _, topic := range []fileBinding{
		manifest.Files.Lefthook,
		manifest.Files.Gitattributes,
		manifest.Files.Dependabot,
	} {
		if hash := hashRepositoryFile(t, topic.Path); hash != topic.SHA256 {
			t.Fatalf("the canonical file %s hashes to %s, want the bound %s", topic.Path, hash, topic.SHA256)
		}
	}
	// The gitignore topic is prefix-mode in the home verifier: the canonical
	// core is a verbatim prefix and the project additions live below the mark.
	gitignore := normalizeLineEndings(readRepositoryDocument(t, manifest.Files.Gitignore.Path))
	const canonicalGitignoreCore = "# Local build and test outputs.\n/.build/\n/dist/\n/coverage/\n/.cache/\n*.coverprofile\n*.test\n*.out\n*.cov\n\n# -- project additions below this line --\n"
	if !strings.HasPrefix(gitignore, canonicalGitignoreCore) {
		t.Fatal("the gitignore does not carry the canonical core as a verbatim prefix with the project-block mark")
	}
	for _, addition := range []string{"cover-github", "*.txt", "!LICENSES/**"} {
		if !strings.Contains(gitignore[len(canonicalGitignoreCore):], addition) {
			t.Fatalf("the gitignore project block misses %q", addition)
		}
	}

	codeowners := readRepositoryDocument(t, manifest.Codeowners.Path)
	if !strings.Contains(codeowners, "* "+manifest.Codeowners.DefaultOwner) {
		t.Fatalf("the ownership file does not bind the default owner %q", manifest.Codeowners.DefaultOwner)
	}
}

func TestConformanceWorkflowBindsTheVerifier(t *testing.T) {
	t.Parallel()

	manifest := readBindingManifest(t)
	content := readRepositoryDocument(t, ".github/workflows/canonical-conformance.yml")
	for _, required := range []string{
		"permissions: {}",
		"name: Canonical conformance",
		"uses: " + manifest.Home.Repository + "/.github/actions/verify-canonical-files@" + manifest.Home.SHA,
		`branches: [main, develop, "release/**", "support/**"]`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("the canonical conformance workflow does not contain %q", required)
		}
	}
}

func TestToolsModulePinsTheCanonicalVerifier(t *testing.T) {
	t.Parallel()

	toolsModule := readRepositoryDocument(t, filepath.Join("tools", "go.mod"))
	for _, required := range []string{
		"github.com/t33n-software/repository-governance/cmd/verify-canonical",
		"github.com/t33n-software/go-quality-authority/cmd/quality-gate",
		"github.com/t33n-software/go-quality-authority/cmd/check-coverage",
	} {
		if !strings.Contains(toolsModule, required) {
			t.Fatalf("tools/go.mod does not pin %q", required)
		}
	}
}

func TestQualityConfigRunsTheCanonicalGateChain(t *testing.T) {
	t.Parallel()

	configuration := readRepositoryDocument(t, "git-governance.quality.json")
	var decoded struct {
		Gates []struct {
			Name    string   `json:"name"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"gates"`
		Defaults json.RawMessage `json:"defaults"`
	}
	if err := json.Unmarshal([]byte(configuration), &decoded); err != nil {
		t.Fatalf("git-governance.quality.json is not valid JSON: %v", err)
	}
	if len(decoded.Defaults) != 0 {
		t.Fatal("the quality configuration must not restate the family defaults: the schema owns the canonical default set")
	}
	if len(decoded.Gates) != 1 {
		t.Fatalf("the quality configuration carries %d gates, want exactly the canonical gate chain", len(decoded.Gates))
	}
	gate := decoded.Gates[0]
	canonicalArgs := "tool -modfile tools/go.mod quality-gate"
	if gate.Name != "full-local-build" || gate.Command != "go" || strings.Join(gate.Args, " ") != canonicalArgs {
		t.Fatalf("the full-local-build gate must invoke the pinned canonical orchestrator (go %s), got %s %s", canonicalArgs, gate.Command, strings.Join(gate.Args, " "))
	}

	for _, copy := range []string{"cmd/build", "cmd/check-coverage"} {
		path := filepath.Join(repositoryRoot(t), copy)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("the chain copy %s must be absent: the canonical gate chain is referenced via the tool pin, never re-implemented per repository", copy)
		}
	}
}

type lifecycleCallerHashRecord struct {
	Home struct {
		Repository string `json:"repository"`
		SHA        string `json:"sha"`
	} `json:"home"`
	Callers []struct {
		Master     string `json:"master"`
		TenantFile string `json:"tenantFile"`
		SHA256     string `json:"sha256"`
	} `json:"callers"`
}

func readLifecycleCallerMaster(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(repositoryRoot(t), "workflows", "github", "callers", "release-lifecycle", name)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return normalizeLineEndings(string(contents))
}

func readLifecycleCallerHashRecord(t *testing.T) lifecycleCallerHashRecord {
	t.Helper()

	path := filepath.Join(repositoryRoot(t), "workflows", "github", "callers", "release-lifecycle", "caller-hashes.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record lifecycleCallerHashRecord
	if err := json.Unmarshal(contents, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

// normalizeLineEndings folds CRLF into LF so the content, hash, and identity
// proofs stay stable regardless of the platform's checkout line endings.
func normalizeLineEndings(contents string) string {
	return strings.ReplaceAll(contents, "\r\n", "\n")
}

func readWorkflow(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(repositoryRoot(t), ".github", "workflows", name)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return normalizeLineEndings(string(contents))
}
