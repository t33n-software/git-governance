package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const (
	defaultGitHubHost      = "github.com"
	defaultOAuthBaseURL    = "https://github.com"
	defaultDeviceInterval  = 5 * time.Second
	credentialExpirySkew   = time.Minute
	maxOAuthResponseBytes  = 1 << 20
	secretStoreSourceLabel = "native-secret-store"
)

var errSessionNotFound = errors.New("GitHub App session not found")

// CredentialTarget identifies the exact GitHub repository for which an API
// credential is requested. A resolver must reject a host mismatch rather than
// attempting to reuse a credential for another GitHub host.
type CredentialTarget struct {
	Host       string
	Owner      string
	Repository string
}

// CredentialResolver obtains a short-lived API token immediately before a
// GitHub API request. Token values are internal adapter data and are never
// included in application reports.
type CredentialResolver interface {
	Resolve(context.Context, CredentialTarget) (string, error)
}

// SessionStore persists only refresh credentials and non-sensitive profile
// metadata in a native operating-system secret store.
type SessionStore interface {
	// LoadActive returns the active session for exactly one host and client
	// ID pair.
	LoadActive(context.Context, string, string) (Session, error)
	// LoadActiveForHost returns the active session for a host without
	// requiring the caller to know its client ID. Exactly one session is
	// active per host; a missing pointer fails closed with
	// errSessionNotFound.
	LoadActiveForHost(context.Context, string) (Session, error)
	SaveActive(context.Context, Session) error
	DeleteActive(context.Context, string, string) error
}

// Session is the persistent, protected portion of a GitHub App login. It
// intentionally has no access-token field: API access tokens live only in the
// resolver's process-memory cache.
type Session struct {
	Host                  string    `json:"host"`
	Account               string    `json:"account"`
	ClientID              string    `json:"clientID"`
	RefreshToken          string    `json:"refreshToken"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
}

// DeviceAuthorization is the short-lived user-facing state of one OAuth
// Device Authorization Grant. DeviceCode must never be rendered to users.
type DeviceAuthorization struct {
	VerificationURI string
	UserCode        string
	ExpiresAt       time.Time
	Interval        time.Duration
}

// LoginRequest contains the non-secret GitHub App client ID and a callback
// used by the CLI to display the user verification instructions.
type LoginRequest struct {
	ClientID              string
	OnDeviceAuthorization func(DeviceAuthorization) error
}

// SessionStatus contains only non-sensitive session metadata suitable for
// human and JSON reports.
type SessionStatus struct {
	Host                  string    `json:"host"`
	Account               string    `json:"account"`
	Source                string    `json:"source"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
	RefreshState          string    `json:"refreshState"`
}

// AuthProvider is the GitHub-specific platform capability used by the
// bootstrap composition root. It keeps GitHub authentication out of ticket,
// branch, commit, and provider-neutral pull-request models.
type AuthProvider interface {
	CredentialResolver
	Login(context.Context, LoginRequest) (SessionStatus, error)
	Status(context.Context) (SessionStatus, error)
	Logout(context.Context) (SessionStatus, error)
}

// AuthOptions provides injectable seams for the GitHub OAuth and API client.
// Production uses the GitHub.com endpoints and a platform-native session
// store; tests provide fake stores, clocks, and HTTP clients.
type AuthOptions struct {
	Store        SessionStore
	Host         string
	OAuthBaseURL string
	APIBaseURL   string
	HTTPClient   *http.Client
	Now          func() time.Time
	Wait         func(context.Context, time.Duration) error
}

// AuthService implements the GitHub App Device Flow, refresh-token lifecycle,
// repository authorization checks, and just-in-time credential resolution.
type AuthService struct {
	store        SessionStore
	host         string
	oauthBaseURL string
	apiBaseURL   string
	client       *http.Client
	now          func() time.Time
	wait         func(context.Context, time.Duration) error

	mutex      sync.Mutex
	cached     map[string]cachedToken
	refreshing map[string]*refreshCall
	authorized map[string]time.Time
}

type cachedToken struct {
	value     string
	expiresAt time.Time
}

type refreshCall struct {
	done  chan struct{}
	token cachedToken
	err   error
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Error           string `json:"error"`
}

type tokenResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	TokenType             string `json:"token_type"`
	Error                 string `json:"error"`
}

type userResponse struct {
	Login string `json:"login"`
}

type installationsResponse struct {
	Installations []installationResponse `json:"installations"`
}

type installationResponse struct {
	ID int64 `json:"id"`
}

type installationRepositoriesResponse struct {
	TotalCount   int                  `json:"total_count"`
	Repositories []repositoryResponse `json:"repositories"`
}

type repositoryResponse struct {
	FullName string `json:"full_name"`
}

// NewAuthService constructs the local GitHub.com App authentication provider.
// No GitHub App private key or client secret is accepted by this client.
func NewAuthService(options AuthOptions) *AuthService {
	store := options.Store
	if store == nil {
		store = newPlatformSessionStore()
	}
	host := strings.TrimSpace(options.Host)
	if host == "" {
		host = defaultGitHubHost
	}
	oauthBaseURL := strings.TrimRight(strings.TrimSpace(options.OAuthBaseURL), "/")
	if oauthBaseURL == "" {
		oauthBaseURL = defaultOAuthBaseURL
	}
	apiBaseURL := strings.TrimRight(strings.TrimSpace(options.APIBaseURL), "/")
	if apiBaseURL == "" {
		apiBaseURL = defaultAPIBaseURL
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	wait := options.Wait
	if wait == nil {
		wait = waitForContext
	}
	return &AuthService{
		store:        store,
		host:         host,
		oauthBaseURL: oauthBaseURL,
		apiBaseURL:   apiBaseURL,
		client:       client,
		now:          now,
		wait:         wait,
		cached:       make(map[string]cachedToken),
		refreshing:   make(map[string]*refreshCall),
		authorized:   make(map[string]time.Time),
	}
}

// Login starts an explicit Device Flow login, waits for user authorization,
// validates the user identity, and persists only the refresh session. The
// public GitHub App client ID is supplied with the request; the persisted
// session becomes the active session for the host and the authority for all
// later credential resolution.
func (service *AuthService) Login(ctx context.Context, request LoginRequest) (SessionStatus, error) {
	if err := service.contextError(ctx, "GitHub App login"); err != nil {
		return SessionStatus{}, err
	}
	requestClientID, err := validateClientID(request.ClientID)
	if err != nil {
		return SessionStatus{}, err
	}
	device, deviceCode, err := service.requestDeviceAuthorization(ctx, requestClientID)
	if err != nil {
		return SessionStatus{}, err
	}
	if request.OnDeviceAuthorization != nil {
		if err := request.OnDeviceAuthorization(device); err != nil {
			return SessionStatus{}, err
		}
	}
	tokens, err := service.pollForTokens(ctx, requestClientID, device, deviceCode)
	if err != nil {
		return SessionStatus{}, err
	}
	account, err := service.lookupAccount(ctx, tokens.AccessToken)
	if err != nil {
		return SessionStatus{}, err
	}
	session := Session{
		Host:                  service.host,
		Account:               account,
		ClientID:              requestClientID,
		RefreshToken:          tokens.RefreshToken,
		RefreshTokenExpiresAt: service.now().Add(time.Duration(tokens.RefreshTokenExpiresIn) * time.Second),
	}
	if err := service.store.SaveActive(ctx, session); err != nil {
		return SessionStatus{}, sessionStoreProblem("save", err)
	}
	service.forgetSession(session)
	return sessionStatus(session, service.now()), nil
}

// Status reads protected session metadata without resolving an access token or
// making a GitHub API call.
func (service *AuthService) Status(ctx context.Context) (SessionStatus, error) {
	if err := service.contextError(ctx, "GitHub App status"); err != nil {
		return SessionStatus{}, err
	}
	session, err := service.loadActiveSession(ctx)
	if err != nil {
		return SessionStatus{}, err
	}
	return sessionStatus(session, service.now()), nil
}

// Logout removes the protected local refresh session. Device-flow refresh
// tokens cannot be revoked by this client without a GitHub App client secret,
// which is intentionally never present on developer machines.
func (service *AuthService) Logout(ctx context.Context) (SessionStatus, error) {
	if err := service.contextError(ctx, "GitHub App logout"); err != nil {
		return SessionStatus{}, err
	}
	session, err := service.loadActiveSession(ctx)
	if err != nil {
		return SessionStatus{}, err
	}
	if err := service.store.DeleteActive(ctx, service.host, session.ClientID); err != nil {
		return SessionStatus{}, sessionStoreProblem("delete", err)
	}
	service.forgetSession(session)
	return sessionStatus(session, service.now()), nil
}

// Resolve returns a valid process-memory access token for exactly one
// GitHub.com repository and verifies that the active App/user session can
// access that repository. It never starts an interactive login.
func (service *AuthService) Resolve(ctx context.Context, target CredentialTarget) (string, error) {
	if err := service.contextError(ctx, "GitHub credential resolution"); err != nil {
		return "", err
	}
	target, err := service.validateTarget(target)
	if err != nil {
		return "", err
	}
	session, err := service.loadActiveSession(ctx)
	if err != nil {
		return "", err
	}
	token, err := service.accessToken(ctx, session)
	if err != nil {
		return "", err
	}
	if err := service.ensureRepositoryAuthorization(ctx, session, token, target); err != nil {
		return "", err
	}
	return token.value, nil
}

func (service *AuthService) requestDeviceAuthorization(
	ctx context.Context,
	clientID string,
) (DeviceAuthorization, string, error) {
	response := deviceCodeResponse{}
	if err := service.oauthFormRequest(ctx, "/login/device/code", url.Values{
		"client_id": {clientID},
	}, &response); err != nil {
		return DeviceAuthorization{}, "", err
	}
	if response.Error != "" {
		return DeviceAuthorization{}, "", oauthProblem(response.Error, "start the GitHub App device authorization again")
	}
	if strings.TrimSpace(response.DeviceCode) == "" || strings.TrimSpace(response.UserCode) == "" ||
		strings.TrimSpace(response.VerificationURI) == "" || response.ExpiresIn <= 0 {
		return DeviceAuthorization{}, "", oauthProblem("invalid_device_response", "retry the GitHub App login")
	}
	verificationURI, err := url.Parse(response.VerificationURI)
	if err != nil || verificationURI.Scheme != "https" || verificationURI.Host == "" {
		return DeviceAuthorization{}, "", oauthProblem("invalid_verification_uri", "retry the GitHub App login")
	}
	interval := time.Duration(response.Interval) * time.Second
	if interval <= 0 {
		interval = defaultDeviceInterval
	}
	return DeviceAuthorization{
		VerificationURI: verificationURI.String(),
		UserCode:        response.UserCode,
		ExpiresAt:       service.now().Add(time.Duration(response.ExpiresIn) * time.Second),
		Interval:        interval,
	}, response.DeviceCode, nil
}

func (service *AuthService) pollForTokens(
	ctx context.Context,
	clientID string,
	device DeviceAuthorization,
	deviceCode string,
) (tokenResponse, error) {
	interval := device.Interval
	for {
		if !service.now().Before(device.ExpiresAt) {
			return tokenResponse{}, oauthProblem("expired_token", "run auth login github again")
		}
		response := tokenResponse{}
		err := service.oauthFormRequest(ctx, "/login/oauth/access_token", url.Values{
			"client_id":   {clientID},
			"device_code": {deviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}, &response)
		if err != nil {
			return tokenResponse{}, err
		}
		switch response.Error {
		case "":
			if err := validateTokenResponse(response); err != nil {
				return tokenResponse{}, err
			}
			return response, nil
		case "authorization_pending":
			if err := service.wait(ctx, interval); err != nil {
				return tokenResponse{}, waitProblem(err)
			}
		case "slow_down":
			interval += defaultDeviceInterval
			if err := service.wait(ctx, interval); err != nil {
				return tokenResponse{}, waitProblem(err)
			}
		default:
			return tokenResponse{}, oauthProblem(response.Error, "run auth login github again")
		}
	}
}

func (service *AuthService) accessToken(ctx context.Context, session Session) (cachedToken, error) {
	key := sessionKey(session.Host, session.Account, session.ClientID)
	now := service.now()
	service.mutex.Lock()
	if cached, found := service.cached[key]; found && tokenUsable(cached, now) {
		service.mutex.Unlock()
		return cached, nil
	}
	if pending, found := service.refreshing[key]; found {
		service.mutex.Unlock()
		select {
		case <-ctx.Done():
			return cachedToken{}, service.contextError(ctx, "GitHub credential refresh")
		case <-pending.done:
			if pending.err != nil {
				return cachedToken{}, pending.err
			}
			return pending.token, nil
		}
	}
	pending := &refreshCall{done: make(chan struct{})}
	service.refreshing[key] = pending
	service.mutex.Unlock()

	token, err := service.refresh(ctx, session)

	service.mutex.Lock()
	if err == nil {
		service.cached[key] = token
		service.dropAuthorizationsForSession(key)
	}
	pending.token = token
	pending.err = err
	delete(service.refreshing, key)
	close(pending.done)
	service.mutex.Unlock()
	if err != nil {
		return cachedToken{}, err
	}
	return token, nil
}

func (service *AuthService) refresh(ctx context.Context, session Session) (cachedToken, error) {
	if !service.now().Before(session.RefreshTokenExpiresAt) {
		return cachedToken{}, oauthProblem("refresh_token_expired", "run auth login github again")
	}
	response := tokenResponse{}
	if err := service.oauthFormRequest(ctx, "/login/oauth/access_token", url.Values{
		"client_id":     {session.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {session.RefreshToken},
	}, &response); err != nil {
		return cachedToken{}, err
	}
	if response.Error != "" {
		return cachedToken{}, oauthProblem(response.Error, "run auth login github again")
	}
	if err := validateTokenResponse(response); err != nil {
		return cachedToken{}, err
	}
	updated := session
	updated.RefreshToken = response.RefreshToken
	updated.RefreshTokenExpiresAt = service.now().Add(time.Duration(response.RefreshTokenExpiresIn) * time.Second)
	if err := service.store.SaveActive(ctx, updated); err != nil {
		return cachedToken{}, sessionStoreProblem("save", err)
	}
	return cachedToken{
		value:     response.AccessToken,
		expiresAt: service.now().Add(time.Duration(response.ExpiresIn) * time.Second),
	}, nil
}

func (service *AuthService) ensureRepositoryAuthorization(
	ctx context.Context,
	session Session,
	token cachedToken,
	target CredentialTarget,
) error {
	authorizationKey := sessionKey(session.Host, session.Account, session.ClientID) + "\x00" + target.Owner + "\x00" + target.Repository
	service.mutex.Lock()
	expiresAt, authorized := service.authorized[authorizationKey]
	service.mutex.Unlock()
	if authorized && expiresAt.Equal(token.expiresAt) {
		return nil
	}
	if err := service.repositoryIsInstalledAndAuthorized(ctx, token.value, target); err != nil {
		return err
	}
	service.mutex.Lock()
	service.authorized[authorizationKey] = token.expiresAt
	service.mutex.Unlock()
	return nil
}

func (service *AuthService) repositoryIsInstalledAndAuthorized(
	ctx context.Context,
	token string,
	target CredentialTarget,
) error {
	installations := installationsResponse{}
	if err := service.githubAPIRequest(ctx, http.MethodGet, "/user/installations?per_page=100", token, &installations); err != nil {
		return err
	}
	expected := strings.ToLower(target.Owner + "/" + target.Repository)
	for _, installation := range installations.Installations {
		if installation.ID <= 0 {
			continue
		}
		page := 1
		for {
			repositories := installationRepositoriesResponse{}
			path := "/user/installations/" + strconv.FormatInt(installation.ID, 10) +
				"/repositories?per_page=100&page=" + strconv.Itoa(page)
			if err := service.githubAPIRequest(ctx, http.MethodGet, path, token, &repositories); err != nil {
				return err
			}
			for _, repository := range repositories.Repositories {
				if strings.EqualFold(strings.TrimSpace(repository.FullName), expected) {
					return nil
				}
			}
			if len(repositories.Repositories) == 0 || page*100 >= repositories.TotalCount {
				break
			}
			page++
		}
	}
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "GitHub repository authorization",
		Expected:    "a GitHub App installation and user access for " + target.Owner + "/" + target.Repository,
		Rule:        "GitHub App credentials must be authorized for the exact remote repository",
		Remediation: "install the GitHub App for the repository and authorize an account that can access it",
	})
}

func (service *AuthService) lookupAccount(ctx context.Context, token string) (string, error) {
	response := userResponse{}
	if err := service.githubAPIRequest(ctx, http.MethodGet, "/user", token, &response); err != nil {
		return "", err
	}
	account := strings.TrimSpace(response.Login)
	if account == "" {
		return "", oauthProblem("invalid_user_response", "run auth login github again")
	}
	return account, nil
}

func (service *AuthService) oauthFormRequest(
	ctx context.Context,
	path string,
	values url.Values,
	target any,
) error {
	endpoint, err := joinHTTPSURL(service.oauthBaseURL, path)
	if err != nil {
		return oauthEndpointProblem(err)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		strings.NewReader(values.Encode()),
	)
	if err != nil {
		return oauthEndpointProblem(err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "git-governance")
	response, err := service.client.Do(request)
	if err != nil {
		return oauthNetworkProblem(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return oauthHTTPProblem(response.StatusCode)
	}
	if err := decodeOAuthResponse(response.Body, target); err != nil {
		return err
	}
	return nil
}

func (service *AuthService) githubAPIRequest(
	ctx context.Context,
	method string,
	path string,
	token string,
	target any,
) error {
	endpoint, err := joinHTTPSURL(service.apiBaseURL, path)
	if err != nil {
		return oauthEndpointProblem(err)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return oauthEndpointProblem(err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", defaultAPIVersion)
	request.Header.Set("User-Agent", "git-governance")
	response, err := service.client.Do(request)
	if err != nil {
		return oauthNetworkProblem(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return oauthHTTPProblem(response.StatusCode)
	}
	return decodeOAuthResponse(response.Body, target)
}

func (service *AuthService) validateTarget(target CredentialTarget) (CredentialTarget, error) {
	target.Host = strings.TrimSpace(target.Host)
	target.Owner = strings.TrimSpace(target.Owner)
	target.Repository = strings.TrimSpace(target.Repository)
	if !strings.EqualFold(target.Host, service.host) {
		return CredentialTarget{}, problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "GitHub remote host",
			Actual:      target.Host,
			Expected:    service.host,
			Rule:        "GitHub App credentials are isolated by GitHub host",
			Remediation: "use a remote hosted by the configured GitHub App host",
		})
	}
	if target.Owner == "" || target.Repository == "" {
		return CredentialTarget{}, problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "GitHub repository",
			Expected:    "a non-empty GitHub owner and repository",
			Rule:        "GitHub App credential resolution requires an exact repository",
			Remediation: "configure a canonical GitHub remote before publishing",
		})
	}
	return target, nil
}

func (service *AuthService) contextError(ctx context.Context, operation string) error {
	if ctx == nil {
		return problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       operation,
			Expected:    "a non-nil context",
			Rule:        "GitHub authentication requires an active context",
			Remediation: "retry with an active context",
		})
	}
	if ctx.Err() == nil {
		return nil
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeOperationCancelled,
		Category:    problem.CategoryCancelled,
		Field:       operation,
		Expected:    "an active context",
		Rule:        "GitHub authentication stops when the caller cancels the command",
		Remediation: "retry with an active context",
	}, ctx.Err())
}

func (service *AuthService) forgetSession(session Session) {
	key := sessionKey(session.Host, session.Account, session.ClientID)
	service.mutex.Lock()
	delete(service.cached, key)
	service.dropAuthorizationsForSession(key)
	service.mutex.Unlock()
}

func (service *AuthService) dropAuthorizationsForSession(prefix string) {
	for key := range service.authorized {
		if strings.HasPrefix(key, prefix+"\x00") {
			delete(service.authorized, key)
		}
	}
}

func validateClientID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "GitHub App client ID",
			Expected:    "a non-empty public GitHub App client ID",
			Rule:        "GitHub App Device Flow requires a public GitHub App client ID",
			Remediation: "enter the public GitHub App client ID when auth login github prompts for it",
		})
	}
	return value, nil
}

// loadActiveSession resolves the single active session for the configured
// host from the protected store. The stored session, not an ambient
// environment value, is the authority for the GitHub App client ID.
func (service *AuthService) loadActiveSession(ctx context.Context) (Session, error) {
	session, err := service.store.LoadActiveForHost(ctx, service.host)
	if err != nil {
		return Session{}, sessionStoreProblem("load", err)
	}
	if err := validateSession(session, service.host, session.ClientID); err != nil {
		return Session{}, err
	}
	return session, nil
}

func validateSession(session Session, expectedHost, expectedClientID string) error {
	if !strings.EqualFold(strings.TrimSpace(session.Host), expectedHost) ||
		strings.TrimSpace(session.Account) == "" ||
		strings.TrimSpace(session.ClientID) != expectedClientID ||
		strings.TrimSpace(session.RefreshToken) == "" ||
		session.RefreshTokenExpiresAt.IsZero() {
		return problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "GitHub App session",
			Expected:    "a complete protected refresh session for the configured GitHub App",
			Rule:        "GitHub App sessions must be isolated by host, account, and configured client ID",
			Remediation: "run auth logout github, then run auth login github again",
		})
	}
	return nil
}

func validateStoredSession(session Session) error {
	if strings.TrimSpace(session.Host) == "" || strings.TrimSpace(session.Account) == "" ||
		strings.TrimSpace(session.ClientID) == "" || strings.TrimSpace(session.RefreshToken) == "" ||
		session.RefreshTokenExpiresAt.IsZero() {
		return errors.New("protected GitHub App session is incomplete")
	}
	return nil
}

func validateTokenResponse(response tokenResponse) error {
	if strings.TrimSpace(response.AccessToken) == "" || strings.TrimSpace(response.RefreshToken) == "" ||
		response.ExpiresIn <= 0 || response.RefreshTokenExpiresIn <= 0 ||
		!strings.EqualFold(strings.TrimSpace(response.TokenType), "bearer") {
		return oauthProblem("invalid_token_response", "run auth login github again")
	}
	return nil
}

func tokenUsable(token cachedToken, now time.Time) bool {
	return strings.TrimSpace(token.value) != "" && token.expiresAt.After(now.Add(credentialExpirySkew))
}

func sessionStatus(session Session, now time.Time) SessionStatus {
	state := "active"
	if !session.RefreshTokenExpiresAt.After(now) {
		state = "expired"
	}
	return SessionStatus{
		Host:                  session.Host,
		Account:               session.Account,
		Source:                secretStoreSourceLabel,
		RefreshTokenExpiresAt: session.RefreshTokenExpiresAt,
		RefreshState:          state,
	}
}

func sessionKey(host, account, clientID string) string {
	return normalizeHost(host) + "\x00" + strings.ToLower(strings.TrimSpace(account)) + "\x00" + strings.TrimSpace(clientID)
}

func sessionScopeKey(host, clientID string) string {
	return normalizeHost(host) + "\x00" + strings.TrimSpace(clientID)
}

func nativeSessionScope(host, clientID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(clientID)))
	return normalizeHost(host) + "." + hex.EncodeToString(digest[:])
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

func waitForContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func joinHTTPSURL(base, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("invalid HTTPS endpoint")
	}
	relative, err := url.Parse(path)
	if err != nil || relative.IsAbs() || relative.Host != "" {
		return "", errors.New("invalid relative API path")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(relative.Path, "/")
	parsed.RawQuery = relative.RawQuery
	parsed.Fragment = ""
	return parsed.String(), nil
}

func decodeOAuthResponse(reader io.Reader, target any) error {
	payload, err := io.ReadAll(io.LimitReader(reader, maxOAuthResponseBytes+1))
	if err != nil || len(payload) > maxOAuthResponseBytes {
		return oauthProblem("invalid_response", "retry after checking the GitHub App connection")
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return oauthProblem("invalid_response", "retry after checking the GitHub App connection")
	}
	return nil
}

func oauthProblem(reason, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "GitHub App authentication",
		Expected:    "a successful GitHub App OAuth Device Flow",
		Rule:        "GitHub App credentials must be acquired through the configured secure OAuth Device Flow",
		Remediation: remediation,
	})
}

func oauthEndpointProblem(cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "GitHub App endpoint",
		Expected:    "a valid HTTPS GitHub App endpoint",
		Rule:        "GitHub App OAuth and API requests must use HTTPS endpoints",
		Remediation: "repair the GitHub App endpoint configuration and retry",
	}, cause)
}

func oauthNetworkProblem(cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       "GitHub App authentication",
		Expected:    "a reachable GitHub App endpoint",
		Rule:        "GitHub App authentication requires a successful HTTPS request",
		Remediation: "check network access and retry the GitHub App operation",
	}, cause)
}

func oauthHTTPProblem(status int) error {
	return problem.New(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "GitHub App authentication",
		Actual:      http.StatusText(status),
		Expected:    "a successful GitHub App response",
		Rule:        "GitHub App authentication must complete without an HTTP authorization error",
		Remediation: "check the GitHub App installation, client ID, and account authorization",
	})
}

func waitProblem(cause error) error {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeOperationCancelled,
			Category:    problem.CategoryCancelled,
			Field:       "GitHub App login",
			Expected:    "an active context while waiting for browser authorization",
			Rule:        "GitHub App Device Flow polling stops when the caller cancels",
			Remediation: "run auth login github again",
		}, cause)
	}
	return oauthNetworkProblem(cause)
}

func sessionStoreProblem(action string, cause error) error {
	if errors.Is(cause, errSessionNotFound) {
		return problem.Wrap(problem.Details{
			Code:        problem.CodeConfigurationUnavailable,
			Category:    problem.CategoryConfig,
			Field:       "GitHub App session",
			Expected:    "a protected local GitHub App session",
			Rule:        "GitHub publishing never falls back to static environment tokens",
			Remediation: "run auth login github in an interactive terminal",
		}, cause)
	}
	return problem.Wrap(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "GitHub App secret store",
		Expected:    "an available native operating-system secret store",
		Rule:        "GitHub refresh credentials must be protected by a native secret store",
		Remediation: "repair the operating-system secret store and retry",
	}, cause)
}

var _ AuthProvider = (*AuthService)(nil)
