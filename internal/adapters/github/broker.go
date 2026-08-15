package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const brokerInstallationTokenPath = "/v1/github/installations/token"

// BrokerOptions configures a managed-workload credential broker. The workload
// identity is not a GitHub credential; the broker validates it before minting
// a repository-bound GitHub App installation token.
type BrokerOptions struct {
	Endpoint         string
	WorkloadIdentity func() string
	HTTPClient       *http.Client
	Now              func() time.Time
}

// BrokerResolver resolves short-lived GitHub App installation tokens for
// managed CI or enterprise workloads. It never reads or stores a GitHub App
// private key, user refresh token, or access token on disk.
type BrokerResolver struct {
	endpoint         string
	workloadIdentity func() string
	client           *http.Client
	now              func() time.Time

	mutex  sync.Mutex
	cached map[string]cachedToken
}

type brokerRequest struct {
	Host       string `json:"host"`
	Owner      string `json:"owner"`
	Repository string `json:"repository"`
}

type brokerResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// NewBrokerResolver constructs a broker-backed resolver. Endpoint validation
// occurs on use so unrelated CLI commands do not require managed CI settings.
func NewBrokerResolver(options BrokerOptions) *BrokerResolver {
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	identity := options.WorkloadIdentity
	if identity == nil {
		identity = func() string { return "" }
	}
	return &BrokerResolver{
		endpoint:         strings.TrimRight(strings.TrimSpace(options.Endpoint), "/"),
		workloadIdentity: identity,
		client:           client,
		now:              now,
		cached:           make(map[string]cachedToken),
	}
}

// Resolve requests a repository-bound installation token from the managed
// broker or returns a still-valid process-memory cached token.
func (resolver *BrokerResolver) Resolve(ctx context.Context, target CredentialTarget) (string, error) {
	if ctx == nil || ctx.Err() != nil {
		return "", brokerContextProblem(ctx)
	}
	target, err := validateBrokerTarget(target)
	if err != nil {
		return "", err
	}
	key := strings.ToLower(target.Host + "\x00" + target.Owner + "\x00" + target.Repository)
	resolver.mutex.Lock()
	if cached, found := resolver.cached[key]; found && tokenUsable(cached, resolver.now()) {
		resolver.mutex.Unlock()
		return cached.value, nil
	}
	resolver.mutex.Unlock()

	identity := strings.TrimSpace(resolver.workloadIdentity())
	if identity == "" {
		return "", problem.New(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "GitHub credential broker workload identity",
			Expected:    "a non-empty managed workload identity token",
			Rule:        "managed GitHub App credentials require broker-authenticated workload identity",
			Remediation: "configure the CI workload identity provider and retry publication",
		})
	}
	endpoint, err := joinHTTPSURL(resolver.endpoint, brokerInstallationTokenPath)
	if err != nil {
		return "", brokerEndpointProblem(err)
	}
	body, _ := json.Marshal(brokerRequest(target))
	// The method, validated HTTPS endpoint, and non-nil context are fixed by
	// this adapter, so request construction cannot fail after the checks above.
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+identity)
	request.Header.Set("User-Agent", "git-governance")
	response, err := resolver.client.Do(request)
	if err != nil {
		return "", brokerNetworkProblem(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", brokerHTTPProblem(response.StatusCode)
	}
	var issued brokerResponse
	if err := decodeOAuthResponse(response.Body, &issued); err != nil {
		return "", err
	}
	if strings.TrimSpace(issued.AccessToken) == "" || !issued.ExpiresAt.After(resolver.now().Add(credentialExpirySkew)) {
		return "", problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "GitHub credential broker response",
			Expected:    "a non-empty short-lived installation token with a future expiration",
			Rule:        "the broker must mint repository-bound short-lived GitHub App installation tokens",
			Remediation: "repair the credential broker response contract and retry publication",
		})
	}
	resolver.mutex.Lock()
	resolver.cached[key] = cachedToken{value: issued.AccessToken, expiresAt: issued.ExpiresAt}
	resolver.mutex.Unlock()
	return issued.AccessToken, nil
}

func validateBrokerTarget(target CredentialTarget) (CredentialTarget, error) {
	target.Host = strings.TrimSpace(target.Host)
	target.Owner = strings.TrimSpace(target.Owner)
	target.Repository = strings.TrimSpace(target.Repository)
	if target.Host == "" || target.Owner == "" || target.Repository == "" {
		return CredentialTarget{}, problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "GitHub credential broker repository",
			Expected:    "a non-empty GitHub host, owner, and repository",
			Rule:        "managed GitHub App tokens must be requested for exactly one repository",
			Remediation: "configure a canonical GitHub remote before publication",
		})
	}
	return target, nil
}

func brokerContextProblem(ctx context.Context) error {
	if ctx == nil {
		return problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       "GitHub credential broker context",
			Expected:    "a non-nil context",
			Rule:        "managed GitHub credential resolution requires an active context",
			Remediation: "retry the publication with an active context",
		})
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeOperationCancelled,
		Category:    problem.CategoryCancelled,
		Field:       "GitHub credential broker",
		Expected:    "an active context",
		Rule:        "managed GitHub credential resolution stops when the caller cancels",
		Remediation: "retry the publication with an active context",
	}, ctx.Err())
}

func brokerEndpointProblem(cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "GitHub credential broker endpoint",
		Expected:    "a valid HTTPS credential broker endpoint",
		Rule:        "managed GitHub App credentials must be requested over HTTPS",
		Remediation: "set a valid GIT_GOVERNANCE_GITHUB_CREDENTIAL_BROKER_URL and retry",
	}, cause)
}

func brokerNetworkProblem(cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "GitHub credential broker",
		Expected:    "a reachable credential broker endpoint",
		Rule:        "managed GitHub App credentials require a successful broker request",
		Remediation: "check workload network access and retry publication",
	}, cause)
}

func brokerHTTPProblem(status int) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "GitHub credential broker",
		Actual:      http.StatusText(status),
		Expected:    "a successful credential broker response",
		Rule:        "the credential broker must authorize the workload and repository before minting a token",
		Remediation: "check workload policy, GitHub App installation, and repository authorization",
	})
}

var _ CredentialResolver = (*BrokerResolver)(nil)
