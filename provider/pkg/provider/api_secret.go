package provider

import (
	"context"
	"fmt"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// ApiSecret manages one API secret of an account. Each account (primary or
// sub) can hold at most two secrets at a time. For rotation without a cut-over
// window, manage two ApiSecret resources (blue and green); changing the value of a
// single one replaces it, revoking the old value first when the account is at the limit.
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
		"Vonage never returns it again, so it is kept in state as a secret. Changing it replaces the resource: when the account already holds two secrets the old one is revoked first, otherwise the new one is created first.")
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
		var ok bool
		owner, id, ok = strings.Cut(req.ID, "/")
		if !ok || owner == "" || id == "" {
			return infer.ReadResponse[ApiSecretArgs, ApiSecretState]{}, fmt.Errorf(
				"invalid ApiSecret ID %q: expected ACCOUNT_API_KEY/SECRET_ID, for example abcd1234/ad6dc56f-07b5-46e1-a527-85530e625800", req.ID)
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
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d, DeleteBeforeReplace: r.needsDeleteBeforeReplace(ctx, req, d)}, nil
}

// needsDeleteBeforeReplace reports whether replacing this secret must revoke the
// old one first. An account holds at most two secrets, so when it already holds two
// (typically its original secret plus this managed one) a create-then-revoke
// replacement would be refused. With only one secret the old one cannot be revoked
// first (Vonage keeps the last one), so the default create-then-delete order is used.
func (r *ApiSecret) needsDeleteBeforeReplace(ctx context.Context, req infer.DiffRequest[ApiSecretArgs, ApiSecretState], d map[string]p.PropertyDiff) bool {
	// Only a changed secret value on the same account needs the old one out of the way;
	// a new owner has its own limit.
	if d["secret"].Kind != p.UpdateReplace || d["accountKey"].Kind == p.UpdateReplace || req.State.OwnerKey == "" {
		return false
	}
	c, err := clientFor(ctx)
	if err == nil {
		var secrets []wireSecret
		if secrets, err = c.ListSecrets(ctx, req.State.OwnerKey); err == nil {
			return len(secrets) >= 2
		}
	}
	p.GetLogger(ctx).Warningf("could not count the API secrets of account %s, replacing the secret with create-before-delete: %v", req.State.OwnerKey, err)
	return false
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

// GetApiSecret looks up one API secret of an account (the value is never returned by Vonage).
type GetApiSecret struct{}

// GetApiSecretArgs are the lookup arguments.
type GetApiSecretArgs struct {
	AccountKey *string `pulumi:"accountKey,optional"`
	SecretID   string  `pulumi:"secretId"`
}

func (a *GetApiSecretArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.AccountKey, "API key of the owning account. Defaults to the provider's account.")
	an.Describe(&a.SecretID, "The secret ID.")
}

// GetApiSecretResult is the metadata of an API secret.
type GetApiSecretResult struct {
	OwnerKey  string `pulumi:"ownerKey"`
	SecretID  string `pulumi:"secretId"`
	CreatedAt string `pulumi:"createdAt"`
}

func (a *GetApiSecretResult) Annotate(an infer.Annotator) {
	an.Describe(&a.OwnerKey, "The account API key that owns the secret.")
	an.Describe(&a.SecretID, "The secret ID.")
	an.Describe(&a.CreatedAt, "ISO 8601 creation timestamp.")
}

func (f *GetApiSecret) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up an API secret's metadata by account and secret ID. Vonage never returns the secret value. Fails if it does not exist.")
}

func (f *GetApiSecret) Invoke(ctx context.Context, req infer.FunctionRequest[GetApiSecretArgs]) (infer.FunctionResponse[GetApiSecretResult], error) {
	owner := ownerKey(ctx, ApiSecretArgs{AccountKey: req.Input.AccountKey})
	if owner == "" || req.Input.SecretID == "" {
		return infer.FunctionResponse[GetApiSecretResult]{}, fmt.Errorf("getApiSecret: secretId and an account (accountKey or the provider apiKey) are required")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetApiSecretResult]{}, err
	}
	s, err := c.GetSecret(ctx, owner, req.Input.SecretID)
	if err != nil {
		return infer.FunctionResponse[GetApiSecretResult]{}, fmt.Errorf("getApiSecret: %w", err)
	}
	if s == nil {
		return infer.FunctionResponse[GetApiSecretResult]{}, fmt.Errorf("getApiSecret: secret %q of account %q was not found", req.Input.SecretID, owner)
	}
	return infer.FunctionResponse[GetApiSecretResult]{Output: GetApiSecretResult{OwnerKey: owner, SecretID: s.ID, CreatedAt: s.CreatedAt}}, nil
}

// GetApiSecrets lists the API secrets of an account.
type GetApiSecrets struct{}

// GetApiSecretsArgs are the lookup arguments.
type GetApiSecretsArgs struct {
	AccountKey *string `pulumi:"accountKey,optional"`
}

func (a *GetApiSecretsArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.AccountKey, "API key of the account. Defaults to the provider's account.")
}

// ApiSecretSummary is one listed secret.
type ApiSecretSummary struct {
	SecretID  string `pulumi:"secretId"`
	CreatedAt string `pulumi:"createdAt"`
}

func (a *ApiSecretSummary) Annotate(an infer.Annotator) {
	an.Describe(&a.SecretID, "The secret ID.")
	an.Describe(&a.CreatedAt, "ISO 8601 creation timestamp.")
}

// GetApiSecretsResult lists an account's secrets.
type GetApiSecretsResult struct {
	OwnerKey string             `pulumi:"ownerKey"`
	Secrets  []ApiSecretSummary `pulumi:"secrets"`
}

func (a *GetApiSecretsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.OwnerKey, "The account API key that was listed.")
	an.Describe(&a.Secrets, "The account's secrets (at most two).")
}

func (f *GetApiSecrets) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the API secrets (IDs and creation times) of an account.")
}

func (f *GetApiSecrets) Invoke(ctx context.Context, req infer.FunctionRequest[GetApiSecretsArgs]) (infer.FunctionResponse[GetApiSecretsResult], error) {
	owner := ownerKey(ctx, ApiSecretArgs{AccountKey: req.Input.AccountKey})
	if owner == "" {
		return infer.FunctionResponse[GetApiSecretsResult]{}, fmt.Errorf("getApiSecrets: set accountKey or the provider apiKey")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetApiSecretsResult]{}, err
	}
	list, err := c.ListSecrets(ctx, owner)
	if err != nil {
		return infer.FunctionResponse[GetApiSecretsResult]{}, fmt.Errorf("getApiSecrets: %w", err)
	}
	out := GetApiSecretsResult{OwnerKey: owner, Secrets: []ApiSecretSummary{}}
	for _, s := range list {
		out.Secrets = append(out.Secrets, ApiSecretSummary{SecretID: s.ID, CreatedAt: s.CreatedAt})
	}
	return infer.FunctionResponse[GetApiSecretsResult]{Output: out}, nil
}
