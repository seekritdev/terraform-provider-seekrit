package client

import (
	"context"
	"fmt"
	"net/url"
)

// ── models (JSON shapes returned by apps/api) ───────────────────────────────

// Application is a seekrit application row.
type Application struct {
	ID        string `json:"id"`
	OrgID     string `json:"orgId"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Group is a reusable, org-scoped secret bag.
type Group struct {
	ID        string `json:"id"`
	OrgID     string `json:"orgId"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Environment is owned by exactly one of an application or a group.
type Environment struct {
	ID            string  `json:"id"`
	ApplicationID *string `json:"applicationId"`
	GroupID       *string `json:"groupId"`
	Name          string  `json:"name"`
	Slug          string  `json:"slug"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

// ServiceToken is the public view of a token (never the secret string or hash).
type ServiceToken struct {
	ID            string  `json:"id"`
	OrgID         string  `json:"orgId"`
	Role          string  `json:"role"`
	EnvironmentID *string `json:"environmentId"`
	Name          string  `json:"name"`
	PublicKeyJwk  string  `json:"publicKeyJwk"`
	ExpiresAt     *string `json:"expiresAt"`
	LastUsedAt    *string `json:"lastUsedAt"`
	RevokedAt     *string `json:"revokedAt"`
	CreatedAt     string  `json:"createdAt"`
}

// EnvironmentKeyGrant is a DEK grant (wrapped DEK) for one principal.
type EnvironmentKeyGrant struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	PrincipalType string `json:"principalType"`
	PrincipalID   string `json:"principalId"`
	WrappedDek    string `json:"wrappedDek"`
	CreatedAt     string `json:"createdAt"`
}

// EnvironmentGroupRef is a group composed into an application environment.
type EnvironmentGroupRef struct {
	GroupID  string `json:"groupId"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Position int64  `json:"position"`
}

// OrgMember carries the public key needed to wrap a DEK grant for a user.
type OrgMember struct {
	UserID       string  `json:"userId"`
	Email        string  `json:"email"`
	Name         *string `json:"name"`
	Role         string  `json:"role"`
	PublicKeyJwk *string `json:"publicKeyJwk"`
}

// ── applications ────────────────────────────────────────────────────────────

func (c *Client) CreateApplication(ctx context.Context, orgID, name, slug string) (*Application, error) {
	var out struct {
		App Application `json:"app"`
	}
	err := c.do(ctx, "POST", fmt.Sprintf("/v1/orgs/%s/apps", orgID),
		map[string]string{"name": name, "slug": slug}, &out)
	if err != nil {
		return nil, err
	}
	return &out.App, nil
}

func (c *Client) GetApplication(ctx context.Context, orgID, appID string) (*Application, error) {
	var out struct {
		App Application `json:"app"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/apps/%s", orgID, appID), nil, &out); err != nil {
		return nil, err
	}
	return &out.App, nil
}

func (c *Client) UpdateApplication(ctx context.Context, orgID, appID, name string) (*Application, error) {
	var out struct {
		App Application `json:"app"`
	}
	err := c.do(ctx, "PATCH", fmt.Sprintf("/v1/orgs/%s/apps/%s", orgID, appID),
		map[string]string{"name": name}, &out)
	if err != nil {
		return nil, err
	}
	return &out.App, nil
}

func (c *Client) DeleteApplication(ctx context.Context, orgID, appID string) error {
	return c.do(ctx, "DELETE", fmt.Sprintf("/v1/orgs/%s/apps/%s", orgID, appID), nil, nil)
}

// ── groups ──────────────────────────────────────────────────────────────────

func (c *Client) CreateGroup(ctx context.Context, orgID, name, slug string) (*Group, error) {
	var out struct {
		Group Group `json:"group"`
	}
	err := c.do(ctx, "POST", fmt.Sprintf("/v1/orgs/%s/groups", orgID),
		map[string]string{"name": name, "slug": slug}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Group, nil
}

func (c *Client) GetGroup(ctx context.Context, orgID, groupID string) (*Group, error) {
	var out struct {
		Group Group `json:"group"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/groups/%s", orgID, groupID), nil, &out); err != nil {
		return nil, err
	}
	return &out.Group, nil
}

func (c *Client) UpdateGroup(ctx context.Context, orgID, groupID, name string) (*Group, error) {
	var out struct {
		Group Group `json:"group"`
	}
	err := c.do(ctx, "PATCH", fmt.Sprintf("/v1/orgs/%s/groups/%s", orgID, groupID),
		map[string]string{"name": name}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Group, nil
}

func (c *Client) DeleteGroup(ctx context.Context, orgID, groupID string) error {
	return c.do(ctx, "DELETE", fmt.Sprintf("/v1/orgs/%s/groups/%s", orgID, groupID), nil, nil)
}

// ── environments ────────────────────────────────────────────────────────────

// CreateAppEnv creates an application-owned environment. wrappedDek is the DEK
// wrapped to the calling principal's public key (see internal/crypto).
func (c *Client) CreateAppEnv(ctx context.Context, orgID, appID, name, slug, wrappedDek string) (*Environment, error) {
	var out struct {
		Environment Environment `json:"environment"`
	}
	err := c.do(ctx, "POST", fmt.Sprintf("/v1/orgs/%s/apps/%s/envs", orgID, appID),
		map[string]string{"name": name, "slug": slug, "wrappedDek": wrappedDek}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Environment, nil
}

// CreateGroupEnv creates a group-owned environment.
func (c *Client) CreateGroupEnv(ctx context.Context, orgID, groupID, name, slug, wrappedDek string) (*Environment, error) {
	var out struct {
		Environment Environment `json:"environment"`
	}
	err := c.do(ctx, "POST", fmt.Sprintf("/v1/orgs/%s/groups/%s/envs", orgID, groupID),
		map[string]string{"name": name, "slug": slug, "wrappedDek": wrappedDek}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Environment, nil
}

func (c *Client) GetEnvironment(ctx context.Context, orgID, envID string) (*Environment, error) {
	var out struct {
		Environment Environment `json:"environment"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/envs/%s", orgID, envID), nil, &out); err != nil {
		return nil, err
	}
	return &out.Environment, nil
}

func (c *Client) UpdateEnvironment(ctx context.Context, orgID, envID, name string) (*Environment, error) {
	var out struct {
		Environment Environment `json:"environment"`
	}
	err := c.do(ctx, "PATCH", fmt.Sprintf("/v1/orgs/%s/envs/%s", orgID, envID),
		map[string]string{"name": name}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Environment, nil
}

func (c *Client) DeleteEnvironment(ctx context.Context, orgID, envID string) error {
	return c.do(ctx, "DELETE", fmt.Sprintf("/v1/orgs/%s/envs/%s", orgID, envID), nil, nil)
}

// GetMyEnvKey returns the calling principal's wrapped DEK for an environment —
// needed to unwrap the DEK before re-wrapping it for a grant.
func (c *Client) GetMyEnvKey(ctx context.Context, orgID, envID string) (string, error) {
	var out struct {
		WrappedDek string `json:"wrappedDek"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/envs/%s/key", orgID, envID), nil, &out); err != nil {
		return "", err
	}
	return out.WrappedDek, nil
}

// ── environment ↔ group composition ─────────────────────────────────────────

func (c *Client) ListEnvGroups(ctx context.Context, orgID, envID string) ([]EnvironmentGroupRef, error) {
	var out struct {
		Groups []EnvironmentGroupRef `json:"groups"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/envs/%s/groups", orgID, envID), nil, &out); err != nil {
		return nil, err
	}
	return out.Groups, nil
}

// LinkEnvGroup composes a group into an app environment. position is applied
// when non-nil; otherwise the server appends after the current max. The upsert
// is idempotent on (envId, groupId).
func (c *Client) LinkEnvGroup(ctx context.Context, orgID, envID, groupID string, position *int64) (*EnvironmentGroupRef, error) {
	body := map[string]any{"groupId": groupID}
	if position != nil {
		body["position"] = *position
	}
	var out struct {
		Group EnvironmentGroupRef `json:"group"`
	}
	if err := c.do(ctx, "POST", fmt.Sprintf("/v1/orgs/%s/envs/%s/groups", orgID, envID), body, &out); err != nil {
		return nil, err
	}
	return &out.Group, nil
}

func (c *Client) UnlinkEnvGroup(ctx context.Context, orgID, envID, groupID string) error {
	return c.do(ctx, "DELETE", fmt.Sprintf("/v1/orgs/%s/envs/%s/groups/%s", orgID, envID, groupID), nil, nil)
}

// ── service tokens ──────────────────────────────────────────────────────────

// CreateTokenInput is the client-computed token registration payload.
type CreateTokenInput struct {
	Name          string  `json:"name"`
	TokenID       string  `json:"tokenId"`
	TokenHash     string  `json:"tokenHash"`
	PublicKeyJwk  string  `json:"publicKeyJwk"`
	Role          string  `json:"role"`
	EnvironmentID *string `json:"environmentId,omitempty"`
	ExpiresAt     *string `json:"expiresAt,omitempty"`
}

func (c *Client) CreateToken(ctx context.Context, orgID string, input CreateTokenInput) (*ServiceToken, error) {
	var out struct {
		Token ServiceToken `json:"token"`
	}
	if err := c.do(ctx, "POST", fmt.Sprintf("/v1/orgs/%s/tokens", orgID), input, &out); err != nil {
		return nil, err
	}
	return &out.Token, nil
}

func (c *Client) ListTokens(ctx context.Context, orgID string) ([]ServiceToken, error) {
	var out struct {
		Tokens []ServiceToken `json:"tokens"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/tokens", orgID), nil, &out); err != nil {
		return nil, err
	}
	return out.Tokens, nil
}

// GetToken finds a token by id via the list endpoint (the API has no per-token
// GET). Returns nil (no error) if not present.
func (c *Client) GetToken(ctx context.Context, orgID, tokenID string) (*ServiceToken, error) {
	tokens, err := c.ListTokens(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for i := range tokens {
		if tokens[i].ID == tokenID {
			return &tokens[i], nil
		}
	}
	return nil, nil
}

func (c *Client) UpdateToken(ctx context.Context, orgID, tokenID, name string) (*ServiceToken, error) {
	var out struct {
		Token ServiceToken `json:"token"`
	}
	err := c.do(ctx, "PATCH", fmt.Sprintf("/v1/orgs/%s/tokens/%s", orgID, tokenID),
		map[string]string{"name": name}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Token, nil
}

func (c *Client) RevokeToken(ctx context.Context, orgID, tokenID string) error {
	return c.do(ctx, "DELETE", fmt.Sprintf("/v1/orgs/%s/tokens/%s", orgID, tokenID), nil, nil)
}

// ── environment key grants ──────────────────────────────────────────────────

func (c *Client) ListEnvKeys(ctx context.Context, orgID, envID string) ([]EnvironmentKeyGrant, error) {
	var out struct {
		Grants []EnvironmentKeyGrant `json:"grants"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/envs/%s/keys", orgID, envID), nil, &out); err != nil {
		return nil, err
	}
	return out.Grants, nil
}

func (c *Client) GrantEnvKey(ctx context.Context, orgID, envID, principalType, principalID, wrappedDek string) (*EnvironmentKeyGrant, error) {
	var out struct {
		Grant EnvironmentKeyGrant `json:"grant"`
	}
	body := map[string]string{
		"principalType": principalType,
		"principalId":   principalID,
		"wrappedDek":    wrappedDek,
	}
	if err := c.do(ctx, "POST", fmt.Sprintf("/v1/orgs/%s/envs/%s/keys", orgID, envID), body, &out); err != nil {
		return nil, err
	}
	return &out.Grant, nil
}

func (c *Client) RevokeEnvKey(ctx context.Context, orgID, envID, grantID string) error {
	return c.do(ctx, "DELETE", fmt.Sprintf("/v1/orgs/%s/envs/%s/keys/%s", orgID, envID, grantID), nil, nil)
}

// ── members ─────────────────────────────────────────────────────────────────

func (c *Client) ListMembers(ctx context.Context, orgID string) ([]OrgMember, error) {
	var out struct {
		Members []OrgMember `json:"members"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/members", orgID), nil, &out); err != nil {
		return nil, err
	}
	return out.Members, nil
}

// ── organization ────────────────────────────────────────────────────────────

// Organization is the org the configured token belongs to.
type Organization struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (c *Client) GetOrganization(ctx context.Context, orgID string) (*Organization, error) {
	var out struct {
		Org Organization `json:"org"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s", orgID), nil, &out); err != nil {
		return nil, err
	}
	return &out.Org, nil
}

// ── listings (the data sources' lookup-by-slug path) ────────────────────────

func (c *Client) ListApplications(ctx context.Context, orgID string) ([]Application, error) {
	var out struct {
		Apps []Application `json:"apps"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/apps", orgID), nil, &out); err != nil {
		return nil, err
	}
	return out.Apps, nil
}

func (c *Client) ListGroups(ctx context.Context, orgID string) ([]Group, error) {
	var out struct {
		Groups []Group `json:"groups"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/groups", orgID), nil, &out); err != nil {
		return nil, err
	}
	return out.Groups, nil
}

// ListAppEnvs lists an application's environments. The API decorates each row
// with `canDecrypt` for the calling principal; we ignore it — Terraform models
// decrypt access as seekrit_environment_key_grant resources, not as a field.
func (c *Client) ListAppEnvs(ctx context.Context, orgID, appID string) ([]Environment, error) {
	var out struct {
		Environments []Environment `json:"environments"`
	}
	err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/apps/%s/envs", orgID, appID), nil, &out)
	if err != nil {
		return nil, err
	}
	return out.Environments, nil
}

func (c *Client) ListGroupEnvs(ctx context.Context, orgID, groupID string) ([]Environment, error) {
	var out struct {
		Environments []Environment `json:"environments"`
	}
	err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/groups/%s/envs", orgID, groupID), nil, &out)
	if err != nil {
		return nil, err
	}
	return out.Environments, nil
}

// ── secrets ─────────────────────────────────────────────────────────────────

// Secret is a secret's metadata plus its ciphertext. The API never holds or
// returns plaintext; decrypting `Ciphertext` needs the environment DEK, which
// only a grant-holding client can unwrap.
type Secret struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Ciphertext    string `json:"ciphertext"`
	Version       int64  `json:"version"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// ListSecrets returns every secret in an environment. There is no per-secret GET
// on the API, so reads go through the list.
func (c *Client) ListSecrets(ctx context.Context, orgID, envID string) ([]Secret, error) {
	var out struct {
		Secrets []Secret `json:"secrets"`
	}
	err := c.do(ctx, "GET", fmt.Sprintf("/v1/orgs/%s/envs/%s/secrets", orgID, envID), nil, &out)
	if err != nil {
		return nil, err
	}
	return out.Secrets, nil
}

// GetSecret finds one secret by name. Returns nil (no error) when absent.
func (c *Client) GetSecret(ctx context.Context, orgID, envID, name string) (*Secret, error) {
	list, err := c.ListSecrets(ctx, orgID, envID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Name == name {
			return &list[i], nil
		}
	}
	return nil, nil
}

// PutSecret stores a secret's ciphertext, creating it or appending a version.
// ciphertext is an `sc1.` blob the caller produced locally (internal/crypto).
func (c *Client) PutSecret(ctx context.Context, orgID, envID, name, ciphertext string) (*Secret, error) {
	var out struct {
		Secret Secret `json:"secret"`
	}
	err := c.do(ctx, "PUT", fmt.Sprintf("/v1/orgs/%s/envs/%s/secrets/%s", orgID, envID, url.PathEscape(name)),
		map[string]string{"ciphertext": ciphertext}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Secret, nil
}

func (c *Client) DeleteSecret(ctx context.Context, orgID, envID, name string) error {
	return c.do(ctx, "DELETE",
		fmt.Sprintf("/v1/orgs/%s/envs/%s/secrets/%s", orgID, envID, url.PathEscape(name)), nil, nil)
}
