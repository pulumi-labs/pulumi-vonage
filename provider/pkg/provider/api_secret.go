package provider

import (
	"context"
	"fmt"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// ApiSecret manages one API secret of an account. Each account (primary or
// sub) can hold at most two secrets at a time, which is what makes zero-downtime
// rotation possible: create the new secret, roll consumers, then remove the old one.
type ApiSecret struct{}

// ApiSecretArgs are the inputs of an ApiSecret.
type ApiSecretArgs struct {
	AccountKey *string `pulumi:"accountKey,optional"`
	Secret     string  `pulumi:"secret" provider:"secret"`
}

func (a *ApiSecretArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.AccountKey, "API key of the account that owns the secret. Defaults to the provider's account; "+
		"set it to a subaccount's apiKey to rotate that subaccount's secrets. Changing it replaces the resource.")
	an.Describe(&a.Secret, "The secret value (8-25 characters with at least one lowercase letter, one uppercase letter and one digit). "+
		"Vonage never returns it again, so it is kept in state as a secret. Changing it replaces the resource (create new, then revoke old).")
}

// ApiSecretState is the persisted state of an ApiSecret.
type ApiSecretState struct {
	ApiSecretArgs

	// SecretID is the server-assigned secret ID.
	SecretID string `pulumi:"secretId"`
	// OwnerKey is the account key the secret belongs to.
	OwnerKey string `pulumi:"ownerKey"`
	// CreatedAt is the creation timestamp.
	CreatedAt string `pulumi:"createdAt"`
}

func (s *ApiSecretState) Annotate(a infer.Annotator) {
	a.Describe(&s.SecretID, "The Vonage-assigned secret ID.")
	a.Describe(&s.OwnerKey, "The account API key that owns the secret.")
	a.Describe(&s.CreatedAt, "ISO 8601 creation timestamp.")
}

func (r *ApiSecret) Annotate(a infer.Annotator) {
	a.Describe(&r, "An API secret for a Vonage account or subaccount. An account can have at most two secrets; "+
		"Vonage refuses to revoke the last remaining one.")
}

var _ infer.CustomResource[ApiSecretArgs, ApiSecretState] = (*ApiSecret)(nil)
var _ infer.CustomCheck[ApiSecretArgs] = (*ApiSecret)(nil)
var _ infer.CustomRead[ApiSecretArgs, ApiSecretState] = (*ApiSecret)(nil)
var _ infer.CustomDiff[ApiSecretArgs, ApiSecretState] = (*ApiSecret)(nil)
var _ infer.CustomUpdate[ApiSecretArgs, ApiSecretState] = (*ApiSecret)(nil)
var _ infer.CustomDelete[ApiSecretState] = (*ApiSecret)(nil)

func (r *ApiSecret) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[ApiSecretArgs], error) {
	args, failures, err := infer.DefaultCheck[ApiSecretArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[ApiSecretArgs]{Inputs: args, Failures: failures}, err
	}
	if !validAPISecret(args.Secret) {
		failures = append(failures, p.CheckFailure{Property: "secret",
			Reason: "secret must be 8-25 characters with at least one lowercase letter, one uppercase letter and one digit"})
	}
	return infer.CheckResponse[ApiSecretArgs]{Inputs: args, Failures: failures}, nil
}

func ownerKey(ctx context.Context, a ApiSecretArgs) string {
	if a.AccountKey != nil && *a.AccountKey != "" {
		return *a.AccountKey
	}
	return providerAPIKey(ctx)
}

func (r *ApiSecret) Create(ctx context.Context, req infer.CreateRequest[ApiSecretArgs]) (infer.CreateResponse[ApiSecretState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[ApiSecretState]{ID: req.Name, Output: ApiSecretState{ApiSecretArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[ApiSecretState]{}, err
	}
	owner := ownerKey(ctx, a)
	s, err := c.CreateSecret(ctx, owner, a.Secret)
	if err != nil {
		return infer.CreateResponse[ApiSecretState]{}, fmt.Errorf("create API secret for account %s: %w", owner, err)
	}
	return infer.CreateResponse[ApiSecretState]{
		ID:     owner + "/" + s.ID,
		Output: ApiSecretState{ApiSecretArgs: a, SecretID: s.ID, OwnerKey: owner, CreatedAt: s.CreatedAt},
	}, nil
}

func (r *ApiSecret) Read(ctx context.Context, req infer.ReadRequest[ApiSecretArgs, ApiSecretState]) (infer.ReadResponse[ApiSecretArgs, ApiSecretState], error) {
	owner, id := req.State.OwnerKey, req.State.SecretID
	if owner == "" || id == "" {
		for i := 0; i < len(req.ID); i++ {
			if req.ID[i] == '/' {
				owner, id = req.ID[:i], req.ID[i+1:]
				break
			}
		}
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.ReadResponse[ApiSecretArgs, ApiSecretState]{}, err
	}
	s, err := c.GetSecret(ctx, owner, id)
	if err != nil {
		return infer.ReadResponse[ApiSecretArgs, ApiSecretState]{}, fmt.Errorf("read API secret %s: %w", req.ID, err)
	}
	if s == nil {
		return infer.ReadResponse[ApiSecretArgs, ApiSecretState]{}, nil
	}
	st := req.State
	st.OwnerKey, st.SecretID, st.CreatedAt = owner, s.ID, s.CreatedAt
	in := req.Inputs
	if in.Secret == "" && st.Secret == "" {
		// Import: Vonage never returns the secret value, so leave it empty (Diff adopts
		// it from the program without replacing) and record a non-default owner.
		if owner != providerAPIKey(ctx) {
			in.AccountKey = &owner
		}
		st.ApiSecretArgs = in
	}
	return infer.ReadResponse[ApiSecretArgs, ApiSecretState]{ID: req.ID, Inputs: in, State: st}, nil
}

// Diff replaces the secret when the owner or the secret value changes, except for
// an imported secret (empty value in state, since Vonage never returns it): the
// program's value is adopted in place via Update, without minting a new secret
// or revoking the existing one.
func (r *ApiSecret) Diff(ctx context.Context, req infer.DiffRequest[ApiSecretArgs, ApiSecretState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	if req.State.OwnerKey != "" && ownerKey(ctx, req.Inputs) != req.State.OwnerKey {
		d["accountKey"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	switch {
	case req.State.Secret == "" && req.Inputs.Secret != "":
		d["secret"] = p.PropertyDiff{Kind: p.Update}
	case req.State.Secret != req.Inputs.Secret:
		d["secret"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

// Update only ever records the adopted secret of an imported resource; it makes no API call.
func (r *ApiSecret) Update(ctx context.Context, req infer.UpdateRequest[ApiSecretArgs, ApiSecretState]) (infer.UpdateResponse[ApiSecretState], error) {
	st := req.State
	st.ApiSecretArgs = req.Inputs
	return infer.UpdateResponse[ApiSecretState]{Output: st}, nil
}

func (r *ApiSecret) Delete(ctx context.Context, req infer.DeleteRequest[ApiSecretState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	// RevokeSecret already treats a 404 (secret revoked out of band) as success.
	if err := c.RevokeSecret(ctx, req.State.OwnerKey, req.State.SecretID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("revoke API secret %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}
