package provider

import (
	"context"
	"fmt"
	"github.com/seekritdev/terraform-provider-seekrit/internal/client"
	"github.com/seekritdev/terraform-provider-seekrit/internal/crypto"
)

func (d *providerData) wrapAppRecipients(ctx context.Context, kind, id string, dek []byte) ([]client.UserGrant, error) {
	recipients, err := d.client.AccessRecipients(ctx, d.orgID, kind, id)
	if err != nil {
		return nil, err
	}
	grants := make([]client.UserGrant, 0, len(recipients))
	for _, recipient := range recipients {
		wrapped, err := crypto.WrapDEK(dek, recipient.PublicKeyJWK)
		if err != nil {
			return nil, err
		}
		grants = append(grants, client.UserGrant{UserID: recipient.UserID, WrappedDEK: wrapped})
	}
	return grants, nil
}

func (d *providerData) wrapGroupAppAccess(ctx context.Context, envID, groupID string) ([]client.AppAccessGrant, error) {
	env, err := d.client.GetEnvironment(ctx, d.orgID, envID)
	if err != nil {
		return nil, err
	}
	if env.ApplicationID == nil {
		return nil, fmt.Errorf("only application environments can compose groups")
	}
	recipients, err := d.client.AccessRecipients(ctx, d.orgID, "apps", *env.ApplicationID)
	if err != nil {
		return nil, err
	}
	result := make([]client.AppAccessGrant, 0)
	if len(recipients) == 0 {
		return result, nil
	}
	envs, err := d.client.ListGroupEnvs(ctx, d.orgID, groupID)
	if err != nil {
		return nil, err
	}
	for _, groupEnv := range envs {
		grants, err := d.client.ListEnvKeys(ctx, d.orgID, groupEnv.ID)
		if err != nil {
			return nil, err
		}
		granted := map[string]bool{}
		for _, grant := range grants {
			if grant.PrincipalType == "user" {
				granted[grant.PrincipalID] = true
			}
		}
		for _, recipient := range recipients {
			if granted[recipient.UserID] {
				continue
			}
			dek, err := d.envDEK(ctx, groupEnv.ID)
			if err != nil {
				return nil, err
			}
			wrapped, err := crypto.WrapDEK(dek, recipient.PublicKeyJWK)
			if err != nil {
				return nil, err
			}
			result = append(result, client.AppAccessGrant{EnvironmentID: groupEnv.ID, UserID: recipient.UserID, WrappedDEK: wrapped})
		}
	}
	return result, nil
}
