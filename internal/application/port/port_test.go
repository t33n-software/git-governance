package port

import (
	"errors"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestQualityStatusValuesRemainStable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		status QualityStatus
		want   string
	}{
		{status: QualityUnconfigured, want: "unconfigured"},
		{status: QualitySkipped, want: "skipped"},
		{status: QualityPassed, want: "passed"},
	}

	for _, testCase := range testCases {
		if string(testCase.status) != testCase.want {
			t.Errorf("QualityStatus %q = %q, want %q", testCase.status, testCase.status, testCase.want)
		}
	}
}

func TestCredentialCapabilitySurface(t *testing.T) {
	t.Parallel()

	for capability, valid := range map[CredentialCapability]bool{
		CapabilityPullRequests: true,
		CapabilityDeployments:  true,
		"":                     false,
		"pull_request":         false,
		"actions":              false,
	} {
		if capability.Valid() != valid {
			t.Fatalf("Valid(%q) = %v, want %v", capability, capability.Valid(), valid)
		}
	}

	carries := map[string]string{"pull_requests": "write", "deployments": "read", "contents": "read"}
	for _, name := range []string{"pull_requests", "deployments", "contents"} {
		if !AppPermissionCarries(carries, name) {
			t.Fatalf("AppPermissionCarries(%q) = false, want true", name)
		}
	}
	if AppPermissionCarries(carries, "actions") {
		t.Fatal("a missing key does not carry the capability")
	}
	if AppPermissionCarries(nil, "pull_requests") {
		t.Fatal("a nil permission map does not carry anything")
	}
	if !AppPermissionCarries(map[string]string{"pull_requests": "read"}, "pull_requests") {
		t.Fatal("the read level carries the capability")
	}
	if AppPermissionCarries(map[string]string{"pull_requests": "admin"}, "pull_requests") {
		t.Fatal("an unknown level does not carry the capability")
	}

	wrapped := problem.Wrap(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "GitHub App session capability",
		Expected:    "a session carrying the requested permission class",
		Rule:        "capability-scoped session selection binds only a carrying app class",
		Remediation: "bind the app class that carries the requested permission",
	}, ErrCapabilitySessionMissing)
	if !errors.Is(wrapped, ErrCapabilitySessionMissing) {
		t.Fatal("the capability-missing sentinel must stay errors.Is-distinguishable through its problem wrapper")
	}
}
