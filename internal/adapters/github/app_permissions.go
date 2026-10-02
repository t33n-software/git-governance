package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const appInstallationPageSize = 100

// appRegistrationResponse is the bounded projection of the public app
// registration: the URL-friendly slug and the permission map of the app
// class.
type appRegistrationResponse struct {
	Slug        string            `json:"slug"`
	Permissions map[string]string `json:"permissions"`
}

// userInstallationsResponse is the bounded projection of the user-to-server
// installation surface. Every installation returned through one session's
// user access token belongs to the app that issued the token, so the response
// names exactly the app identity of the bound session.
type userInstallationsResponse struct {
	Installations []struct {
		AppSlug string `json:"app_slug"`
	} `json:"installations"`
}

// InspectAppPermissions resolves the app identity of the bound session and
// reads its public registration fresh per invocation. The snapshot classifies
// which platform surface read classes the configured provider app carries:
// the slug resolution follows the session identity through the user
// installation surface, and the permission map comes from the public
// registration — no credential extension and no cached capability state.
func (publisher *Publisher) InspectAppPermissions(
	ctx context.Context,
	query port.AppPermissionQuery,
) (port.AppPermissionSnapshot, error) {
	apiBase, repository, err := publisher.lifecycleTarget(query.RemoteURL)
	if err != nil {
		return port.AppPermissionSnapshot{}, err
	}
	slug, err := publisher.sessionAppSlug(ctx, apiBase, repository)
	if err != nil {
		return port.AppPermissionSnapshot{}, err
	}
	registration, err := publisher.appRegistration(ctx, apiBase, repository, slug)
	if err != nil {
		return port.AppPermissionSnapshot{}, err
	}
	permissions := registration.Permissions
	if permissions == nil {
		permissions = map[string]string{}
	}
	return port.AppPermissionSnapshot{Slug: registration.Slug, Permissions: permissions}, nil
}

// sessionAppSlug resolves the URL-friendly app slug of the bound session's
// app through the user installation surface the session token already
// authorizes.
func (publisher *Publisher) sessionAppSlug(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
) (string, error) {
	endpoint := appEndpoint(apiBase, "/user/installations", url.Values{"per_page": {strconv.Itoa(appInstallationPageSize)}})
	response, err := publisher.request(ctx, repository, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return "", appPermissionProblem(
			"GitHub App installation discovery",
			http.StatusText(response.StatusCode),
			"a session authorized to list its own GitHub App installations",
			"verify the GitHub App session and its installation authorization before allocating ticket numbers",
		)
	}
	var installations userInstallationsResponse
	if err := decodeResponse(response.Body, &installations); err != nil {
		return "", err
	}
	for _, installation := range installations.Installations {
		if installation.AppSlug != "" {
			return installation.AppSlug, nil
		}
	}
	return "", appPermissionProblem(
		"GitHub App installation discovery",
		"no app installation for the bound session",
		"the URL-friendly slug of the configured provider app",
		"verify that the session belongs to the configured provider GitHub App before allocating ticket numbers",
	)
}

// appRegistration reads the public registration of one app slug. Public apps
// are readable without authentication and private apps through the bound
// session; both paths send the resolved session credential.
func (publisher *Publisher) appRegistration(
	ctx context.Context,
	apiBase *url.URL,
	repository repositoryRef,
	slug string,
) (appRegistrationResponse, error) {
	endpoint := appEndpoint(apiBase, "/apps/"+url.PathEscape(slug), nil)
	response, err := publisher.request(ctx, repository, http.MethodGet, endpoint, nil)
	if err != nil {
		return appRegistrationResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return appRegistrationResponse{}, appPermissionProblem(
			"GitHub App permission registration",
			http.StatusText(response.StatusCode),
			"a readable public app registration for "+slug,
			"verify the app slug of the bound session before allocating ticket numbers",
		)
	}
	var registration appRegistrationResponse
	if err := decodeResponse(response.Body, &registration); err != nil {
		return appRegistrationResponse{}, err
	}
	if registration.Slug == "" {
		return appRegistrationResponse{}, appPermissionProblem(
			"GitHub App permission registration",
			"a registration response without an app slug",
			"a bounded valid registration naming its slug and permissions",
			"report the malformed registration response before allocating ticket numbers",
		)
	}
	return registration, nil
}

// appEndpoint builds a non-repository API endpoint below the API base: the
// app-level surfaces live outside the /repos/ tree.
func appEndpoint(base *url.URL, path string, query url.Values) *url.URL {
	endpoint := *base
	endpoint.Path = strings.TrimRight(base.Path, "/") + path
	endpoint.RawQuery = ""
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	return &endpoint
}

func appPermissionProblem(field, actual, expected, remediation string) error {
	return problem.New(problem.Details{
		Code:        problem.CodeExternalCommandFailed,
		Category:    problem.CategoryExternal,
		Field:       field,
		Actual:      actual,
		Expected:    expected,
		Rule:        "the provider app permission discovery reads the public app registration fresh per invocation",
		Remediation: remediation,
	})
}

var _ port.AppPermissionInspector = (*Publisher)(nil)
