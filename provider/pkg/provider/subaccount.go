// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// SubAccount manages a Vonage subaccount under the primary account used to
// authenticate the provider. Vonage has no API to delete a subaccount; the
// only teardown is suspension, which is opt-in via suspendOnDelete.
type SubAccount struct{}

// SubAccountArgs are the inputs of a SubAccount.
type SubAccountArgs struct {
	Name                     string  `pulumi:"name"`
	Secret                   *string `pulumi:"secret,optional" provider:"secret"`
	UsePrimaryAccountBalance *bool   `pulumi:"usePrimaryAccountBalance,optional"`
	Suspended                *bool   `pulumi:"suspended,optional"`
	SuspendOnDelete          *bool   `pulumi:"suspendOnDelete,optional"`
}

func (a *SubAccountArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "Subaccount name (1-80 characters).")
	an.Describe(&a.Secret, "API secret for the subaccount, set at creation only (8-25 characters with at least one "+
		"lowercase letter, one uppercase letter and one digit). When omitted Vonage generates one, exposed as the "+
		"generatedSecret output. Create-only: the Vonage API cannot change it on an existing subaccount, so later "+
		"changes are ignored (no diff, no update); manage rotation with the ApiSecret resource.")
	an.Describe(&a.UsePrimaryAccountBalance, "Whether the subaccount draws on the primary account balance (server default true).")
	an.Describe(&a.Suspended, "Whether the subaccount is suspended.")
	an.Describe(&a.SuspendOnDelete, "Vonage cannot delete subaccounts. When true, destroying this resource suspends "+
		"the subaccount; when false or unset it is only removed from Pulumi state and stays active.")
}

// SubAccountState is the persisted state of a SubAccount.
type SubAccountState struct {
	SubAccountArgs

	// APIKey is the subaccount's API key and the resource ID.
	APIKey string `pulumi:"apiKey"`
	// PrimaryAccountAPIKey is the API key of the owning primary account.
	PrimaryAccountAPIKey string `pulumi:"primaryAccountApiKey"`
	// CreatedAt is the creation timestamp.
	CreatedAt string `pulumi:"createdAt"`
	// GeneratedSecret is the API secret returned once at creation.
	GeneratedSecret *string `pulumi:"generatedSecret,optional" provider:"secret"`
}

func (s *SubAccountState) Annotate(a infer.Annotator) {
	a.Describe(&s.APIKey, "The subaccount API key (also the resource ID).")
	a.Describe(&s.PrimaryAccountAPIKey, "API key of the primary account that owns this subaccount.")
	a.Describe(&s.CreatedAt, "ISO 8601 creation timestamp.")
	a.Describe(&s.GeneratedSecret, "The subaccount API secret as returned by Vonage at creation. Stored as a secret.")
}

func (r *SubAccount) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Vonage subaccount under the primary account. Subaccounts cannot be deleted through the API, "+
		"only suspended (see suspendOnDelete). Requires primary account credentials.")
}

var _ infer.CustomResource[SubAccountArgs, SubAccountState] = (*SubAccount)(nil)
var _ infer.CustomCheck[SubAccountArgs] = (*SubAccount)(nil)
var _ infer.CustomRead[SubAccountArgs, SubAccountState] = (*SubAccount)(nil)
var _ infer.CustomUpdate[SubAccountArgs, SubAccountState] = (*SubAccount)(nil)
var _ infer.CustomDiff[SubAccountArgs, SubAccountState] = (*SubAccount)(nil)
var _ infer.CustomDelete[SubAccountState] = (*SubAccount)(nil)

func (r *SubAccount) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[SubAccountArgs], error) {
	args, failures, err := infer.DefaultCheck[SubAccountArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[SubAccountArgs]{Inputs: args, Failures: failures}, err
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(args.Name)); n < 1 || utf8.RuneCountInString(args.Name) > 80 {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must be 1-80 characters"})
	}
	if args.Secret != nil && !validAPISecret(*args.Secret) {
		failures = append(failures, p.CheckFailure{Property: "secret",
			Reason: "secret must be 8-25 characters with at least one lowercase letter, one uppercase letter and one digit"})
	}
	return infer.CheckResponse[SubAccountArgs]{Inputs: args, Failures: failures}, nil
}

func subStateFrom(args SubAccountArgs, s *wireSubaccount, generated *string) SubAccountState {
	return SubAccountState{
		SubAccountArgs: args, APIKey: s.APIKey, PrimaryAccountAPIKey: s.PrimaryAccountAPIKey,
		CreatedAt: s.CreatedAt, GeneratedSecret: generated,
	}
}

func (r *SubAccount) Create(ctx context.Context, req infer.CreateRequest[SubAccountArgs]) (infer.CreateResponse[SubAccountState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[SubAccountState]{ID: "", Output: SubAccountState{SubAccountArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[SubAccountState]{}, err
	}
	s, err := c.CreateSubaccount(ctx, subaccountCreate{Name: a.Name, Secret: a.Secret, UsePrimaryAccountBalance: a.UsePrimaryAccountBalance})
	if err != nil {
		return infer.CreateResponse[SubAccountState]{}, fmt.Errorf("create subaccount %q: %w", a.Name, err)
	}
	var gen *string
	if s.Secret != "" {
		gen = strPtr(s.Secret)
	}
	st := subStateFrom(a, s, gen)
	if a.Suspended != nil && *a.Suspended != s.Suspended {
		s2, err := c.PatchSubaccount(ctx, s.APIKey, subaccountPatch{Suspended: a.Suspended})
		if err != nil {
			return infer.CreateResponse[SubAccountState]{ID: s.APIKey, Output: st},
				infer.ResourceInitFailedError{Reasons: []string{fmt.Sprintf("subaccount created but setting suspended failed: %v", err)}}
		}
		st = subStateFrom(a, s2, gen)
	}
	return infer.CreateResponse[SubAccountState]{ID: s.APIKey, Output: st}, nil
}

func (r *SubAccount) Read(ctx context.Context, req infer.ReadRequest[SubAccountArgs, SubAccountState]) (infer.ReadResponse[SubAccountArgs, SubAccountState], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.ReadResponse[SubAccountArgs, SubAccountState]{}, err
	}
	s, err := c.GetSubaccount(ctx, req.ID)
	if err != nil {
		return infer.ReadResponse[SubAccountArgs, SubAccountState]{}, fmt.Errorf("read subaccount %s: %w", req.ID, err)
	}
	if s == nil {
		return infer.ReadResponse[SubAccountArgs, SubAccountState]{}, nil
	}
	args := req.Inputs
	args.Name = s.Name
	// An unset input means the server default (usePrimaryAccountBalance true, suspended
	// false). Keep it unset while the server agrees with the default, and surface any
	// other server value so a subaccount suspended (or detached from the primary
	// balance) outside Pulumi shows up as drift and import adopts it.
	if args.UsePrimaryAccountBalance != nil || !s.UsePrimaryAccountBalance {
		v := s.UsePrimaryAccountBalance
		args.UsePrimaryAccountBalance = &v
	}
	if args.Suspended != nil || s.Suspended {
		v := s.Suspended
		args.Suspended = &v
	}
	return infer.ReadResponse[SubAccountArgs, SubAccountState]{ID: req.ID, Inputs: args, State: subStateFrom(args, s, req.State.GeneratedSecret)}, nil
}

// Diff ignores secret: the Vonage subaccount PATCH endpoint only accepts name,
// use_primary_account_balance and suspended, so a changed secret can never reach
// Vonage. It is create-only; rotate with the ApiSecret resource.
func (r *SubAccount) Diff(ctx context.Context, req infer.DiffRequest[SubAccountArgs, SubAccountState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.SubAccountArgs
	if in.Name != old.Name {
		d["name"] = p.PropertyDiff{Kind: p.Update}
	}
	if effective(in.UsePrimaryAccountBalance, true) != effective(old.UsePrimaryAccountBalance, true) {
		d["usePrimaryAccountBalance"] = p.PropertyDiff{Kind: p.Update}
	}
	if effective(in.Suspended, false) != effective(old.Suspended, false) {
		d["suspended"] = p.PropertyDiff{Kind: p.Update}
	}
	if ptrDiffers(in.SuspendOnDelete, old.SuspendOnDelete) {
		d["suspendOnDelete"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *SubAccount) Update(ctx context.Context, req infer.UpdateRequest[SubAccountArgs, SubAccountState]) (infer.UpdateResponse[SubAccountState], error) {
	a := req.Inputs
	a.Secret = req.State.Secret // create-only: never record a secret Vonage did not receive
	if req.DryRun {
		st := req.State
		st.SubAccountArgs = a
		return infer.UpdateResponse[SubAccountState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[SubAccountState]{}, err
	}
	// Unset means the server default, so send the effective values: removing
	// suspended: true from the program must unsuspend the subaccount.
	ub, susp := effective(a.UsePrimaryAccountBalance, true), effective(a.Suspended, false)
	patch := subaccountPatch{Name: &a.Name, UsePrimaryAccountBalance: &ub, Suspended: &susp}
	s, err := c.PatchSubaccount(ctx, req.ID, patch)
	if err != nil {
		return infer.UpdateResponse[SubAccountState]{}, fmt.Errorf("update subaccount %s: %w", req.ID, err)
	}
	return infer.UpdateResponse[SubAccountState]{Output: subStateFrom(a, s, req.State.GeneratedSecret)}, nil
}

func (r *SubAccount) Delete(ctx context.Context, req infer.DeleteRequest[SubAccountState]) (infer.DeleteResponse, error) {
	if req.State.SuspendOnDelete == nil || !*req.State.SuspendOnDelete {
		return infer.DeleteResponse{}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	t := true
	if _, err := c.PatchSubaccount(ctx, req.ID, subaccountPatch{Suspended: &t}); err != nil {
		if isNotFound(err) {
			return infer.DeleteResponse{}, nil
		}
		return infer.DeleteResponse{}, fmt.Errorf("suspend subaccount %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// effective returns the value of an optional bool, or def when it is unset.
func effective(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// ptrDiffers reports whether two optional values differ: one set and the other
// not, or both set to different values.
func ptrDiffers[T comparable](a, b *T) bool {
	if (a == nil) != (b == nil) {
		return true
	}
	return a != nil && *a != *b
}

// GetSubAccount looks up a subaccount of the provider's primary account by API key.
type GetSubAccount struct{}

// GetSubAccountArgs are the lookup arguments.
type GetSubAccountArgs struct {
	SubAccountAPIKey string `pulumi:"subAccountApiKey"`
}

func (a *GetSubAccountArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.SubAccountAPIKey, "API key of the subaccount.")
}

// GetSubAccountResult is a subaccount's configuration.
type GetSubAccountResult struct {
	APIKey                   string   `pulumi:"apiKey"`
	Name                     string   `pulumi:"name"`
	PrimaryAccountAPIKey     string   `pulumi:"primaryAccountApiKey"`
	UsePrimaryAccountBalance bool     `pulumi:"usePrimaryAccountBalance"`
	Suspended                bool     `pulumi:"suspended"`
	CreatedAt                string   `pulumi:"createdAt"`
	Balance                  *float64 `pulumi:"balance,optional"`
	CreditLimit              *float64 `pulumi:"creditLimit,optional"`
}

func (a *GetSubAccountResult) Annotate(an infer.Annotator) {
	an.Describe(&a.APIKey, "Subaccount API key.")
	an.Describe(&a.Name, "Subaccount name.")
	an.Describe(&a.PrimaryAccountAPIKey, "API key of the owning primary account.")
	an.Describe(&a.UsePrimaryAccountBalance, "Whether the subaccount draws on the primary account balance.")
	an.Describe(&a.Suspended, "Whether the subaccount is suspended.")
	an.Describe(&a.CreatedAt, "ISO 8601 creation timestamp.")
	an.Describe(&a.Balance, "Subaccount balance, unset when it uses the primary balance.")
	an.Describe(&a.CreditLimit, "Subaccount credit limit, unset when it uses the primary balance.")
}

func (f *GetSubAccount) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a subaccount of the provider's primary account by API key. Fails if it does not exist.")
}

func (f *GetSubAccount) Invoke(ctx context.Context, req infer.FunctionRequest[GetSubAccountArgs]) (infer.FunctionResponse[GetSubAccountResult], error) {
	key := req.Input.SubAccountAPIKey
	if key == "" {
		return infer.FunctionResponse[GetSubAccountResult]{}, fmt.Errorf("getSubAccount: subAccountApiKey must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetSubAccountResult]{}, err
	}
	s, err := c.GetSubaccount(ctx, key)
	if err != nil {
		return infer.FunctionResponse[GetSubAccountResult]{}, fmt.Errorf("getSubAccount: %w", err)
	}
	if s == nil {
		return infer.FunctionResponse[GetSubAccountResult]{}, fmt.Errorf("getSubAccount: subaccount %q was not found", key)
	}
	return infer.FunctionResponse[GetSubAccountResult]{Output: GetSubAccountResult{
		APIKey: s.APIKey, Name: s.Name, PrimaryAccountAPIKey: s.PrimaryAccountAPIKey,
		UsePrimaryAccountBalance: s.UsePrimaryAccountBalance, Suspended: s.Suspended, CreatedAt: s.CreatedAt,
		Balance: s.Balance, CreditLimit: s.CreditLimit,
	}}, nil
}
