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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// AccountSettings manages the default SMS webhooks of the Vonage account the provider
// authenticates as. It is a singleton: the account always has one settings record, so
// creating the resource adopts it and the ID is the API key. Only the callback URLs the
// program sets are managed; the rate limits are read-only outputs.
type AccountSettings struct{}

// AccountSettingsArgs are the inputs of AccountSettings.
type AccountSettingsArgs struct {
	MoCallBackURL *string `pulumi:"moCallBackUrl,optional"`
	DrCallBackURL *string `pulumi:"drCallBackUrl,optional"`
}

func (a *AccountSettingsArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.MoCallBackURL, "Default webhook URL for inbound SMS (mo = mobile originated). Vonage requests the URL "+
		"and only saves it when it answers 200. When unset the resource does not manage it, and removing it from a "+
		"program that managed it clears the URL on the account.")
	an.Describe(&a.DrCallBackURL, "Default webhook URL for SMS delivery receipts (dr = delivery receipt). Vonage requests "+
		"the URL and only saves it when it answers 200. When unset the resource does not manage it, and removing it "+
		"from a program that managed it clears the URL on the account.")
}

// AccountSettingsState is the persisted state of AccountSettings.
type AccountSettingsState struct {
	AccountSettingsArgs

	// MaxOutboundRequest is the account's outbound SMS request limit.
	MaxOutboundRequest *int `pulumi:"maxOutboundRequest,optional"`
	// MaxInboundRequest is the account's inbound SMS request limit.
	MaxInboundRequest *int `pulumi:"maxInboundRequest,optional"`
	// MaxCallsPerSecond is the account's API calls per second limit.
	MaxCallsPerSecond *int `pulumi:"maxCallsPerSecond,optional"`
}

func (s *AccountSettingsState) Annotate(a infer.Annotator) {
	a.Describe(&s.MaxOutboundRequest, "Maximum outbound SMS requests per second Vonage allows the account. Read-only.")
	a.Describe(&s.MaxInboundRequest, "Maximum inbound SMS requests per second Vonage allows the account. Read-only.")
	a.Describe(&s.MaxCallsPerSecond, "Maximum API calls per second Vonage allows the account. Read-only.")
}

func (r *AccountSettings) Annotate(a infer.Annotator) {
	a.Describe(&r, "The default SMS webhooks of the Vonage account (POST /account/settings). A singleton: there is "+
		"exactly one per account, so create adopts it, import ID is the account API key, and destroy clears only the "+
		"URLs this resource manages. Vonage requests each URL and rejects it unless it answers 200.")
}

// flexInt decodes a JSON number or numeric string, since Vonage's account endpoints mix both.
type flexInt struct {
	v  int
	ok bool
}

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		return nil
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("not a number: %s", string(bytes.TrimSpace(b)))
	}
	f.v, f.ok = int(n), true
	return nil
}

func (f flexInt) ptr() *int {
	if !f.ok {
		return nil
	}
	v := f.v
	return &v
}

type wireAccountSettings struct {
	MoCallBackURL      string  `json:"mo-callback-url"`
	DrCallBackURL      string  `json:"dr-callback-url"`
	MaxOutboundRequest flexInt `json:"max-outbound-request"`
	MaxInboundRequest  flexInt `json:"max-inbound-request"`
	MaxCallsPerSecond  flexInt `json:"max-calls-per-second"`
}

// postAccountSettings calls POST /account/settings. With no form fields Vonage returns the
// current settings without changing them (the Vonage Node SDK documents the response as
// "the updated or current settings"); with fields it applies them and returns the result.
// An empty value clears a URL. POST is never retried by the client, so a write is not replayed.
func (c *Client) postAccountSettings(ctx context.Context, form url.Values) (*wireAccountSettings, error) {
	if form == nil {
		form = url.Values{}
	}
	var out wireAccountSettings
	if err := c.restForm(ctx, http.MethodPost, "/account/settings", nil, form, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

var _ infer.CustomResource[AccountSettingsArgs, AccountSettingsState] = (*AccountSettings)(nil)
var _ infer.CustomCheck[AccountSettingsArgs] = (*AccountSettings)(nil)
var _ infer.CustomRead[AccountSettingsArgs, AccountSettingsState] = (*AccountSettings)(nil)
var _ infer.CustomUpdate[AccountSettingsArgs, AccountSettingsState] = (*AccountSettings)(nil)
var _ infer.CustomDiff[AccountSettingsArgs, AccountSettingsState] = (*AccountSettings)(nil)
var _ infer.CustomDelete[AccountSettingsState] = (*AccountSettings)(nil)

func (r *AccountSettings) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[AccountSettingsArgs], error) {
	args, failures, err := infer.DefaultCheck[AccountSettingsArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[AccountSettingsArgs]{Inputs: args, Failures: failures}, err
	}
	if args.MoCallBackURL == nil && args.DrCallBackURL == nil {
		failures = append(failures, p.CheckFailure{Property: "moCallBackUrl",
			Reason: "set moCallBackUrl or drCallBackUrl: a resource that manages neither has nothing to do"})
	}
	for name, v := range map[string]*string{"moCallBackUrl": args.MoCallBackURL, "drCallBackUrl": args.DrCallBackURL} {
		if v == nil {
			continue
		}
		if *v == "" {
			failures = append(failures, p.CheckFailure{Property: name,
				Reason: name + " must not be empty: omit it to leave the URL unmanaged"})
			continue
		}
		if u, err := url.Parse(*v); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			failures = append(failures, p.CheckFailure{Property: name, Reason: name + " must be an absolute http or https URL"})
		}
	}
	return infer.CheckResponse[AccountSettingsArgs]{Inputs: args, Failures: failures}, nil
}

func accountSettingsState(args AccountSettingsArgs, w *wireAccountSettings) AccountSettingsState {
	return AccountSettingsState{
		AccountSettingsArgs: args,
		MaxOutboundRequest:  w.MaxOutboundRequest.ptr(),
		MaxInboundRequest:   w.MaxInboundRequest.ptr(),
		MaxCallsPerSecond:   w.MaxCallsPerSecond.ptr(),
	}
}

func accountSettingsForm(mo, dr *string) url.Values {
	f := url.Values{}
	if mo != nil {
		f.Set("moCallBackUrl", *mo)
	}
	if dr != nil {
		f.Set("drCallBackUrl", *dr)
	}
	return f
}

func (r *AccountSettings) Create(ctx context.Context, req infer.CreateRequest[AccountSettingsArgs]) (infer.CreateResponse[AccountSettingsState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[AccountSettingsState]{Output: AccountSettingsState{AccountSettingsArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[AccountSettingsState]{}, err
	}
	w, err := c.postAccountSettings(ctx, accountSettingsForm(a.MoCallBackURL, a.DrCallBackURL))
	if err != nil {
		return infer.CreateResponse[AccountSettingsState]{}, fmt.Errorf("update account settings: %w", err)
	}
	return infer.CreateResponse[AccountSettingsState]{ID: c.apiKey, Output: accountSettingsState(a, w)}, nil
}

func (r *AccountSettings) Read(ctx context.Context, req infer.ReadRequest[AccountSettingsArgs, AccountSettingsState]) (infer.ReadResponse[AccountSettingsArgs, AccountSettingsState], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.ReadResponse[AccountSettingsArgs, AccountSettingsState]{}, err
	}
	if req.ID != c.apiKey {
		return infer.ReadResponse[AccountSettingsArgs, AccountSettingsState]{}, fmt.Errorf(
			"invalid AccountSettings ID %q: it is the API key of the account the provider authenticates as (%q)", req.ID, c.apiKey)
	}
	w, err := c.postAccountSettings(ctx, nil)
	if err != nil {
		return infer.ReadResponse[AccountSettingsArgs, AccountSettingsState]{}, fmt.Errorf("read account settings: %w", err)
	}
	args := req.Inputs
	imported := args.MoCallBackURL == nil && args.DrCallBackURL == nil
	// A managed URL shows the server value (drift). An unmanaged one stays unset, except on
	// import where whatever the account has becomes the program.
	if args.MoCallBackURL != nil || (imported && w.MoCallBackURL != "") {
		v := w.MoCallBackURL
		args.MoCallBackURL = &v
	}
	if args.DrCallBackURL != nil || (imported && w.DrCallBackURL != "") {
		v := w.DrCallBackURL
		args.DrCallBackURL = &v
	}
	return infer.ReadResponse[AccountSettingsArgs, AccountSettingsState]{ID: req.ID, Inputs: args, State: accountSettingsState(args, w)}, nil
}

func (r *AccountSettings) Diff(ctx context.Context, req infer.DiffRequest[AccountSettingsArgs, AccountSettingsState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.AccountSettingsArgs
	if ptrDiffers(in.MoCallBackURL, old.MoCallBackURL) {
		d["moCallBackUrl"] = p.PropertyDiff{Kind: p.Update}
	}
	if ptrDiffers(in.DrCallBackURL, old.DrCallBackURL) {
		d["drCallBackUrl"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

// changedURL is the form value for one URL on update: the new value when set, an empty
// string to clear a URL the program no longer manages, and nothing when it never did.
func changedURL(in, old *string) *string {
	if in != nil {
		return in
	}
	if old != nil {
		e := ""
		return &e
	}
	return nil
}

func (r *AccountSettings) Update(ctx context.Context, req infer.UpdateRequest[AccountSettingsArgs, AccountSettingsState]) (infer.UpdateResponse[AccountSettingsState], error) {
	a := req.Inputs
	if req.DryRun {
		st := req.State
		st.AccountSettingsArgs = a
		return infer.UpdateResponse[AccountSettingsState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[AccountSettingsState]{}, err
	}
	old := req.State.AccountSettingsArgs
	w, err := c.postAccountSettings(ctx, accountSettingsForm(changedURL(a.MoCallBackURL, old.MoCallBackURL), changedURL(a.DrCallBackURL, old.DrCallBackURL)))
	if err != nil {
		return infer.UpdateResponse[AccountSettingsState]{}, fmt.Errorf("update account settings: %w", err)
	}
	return infer.UpdateResponse[AccountSettingsState]{Output: accountSettingsState(a, w)}, nil
}

// Delete clears the URLs this resource manages. The settings record itself cannot be
// removed, and URLs the program never managed are left alone.
func (r *AccountSettings) Delete(ctx context.Context, req infer.DeleteRequest[AccountSettingsState]) (infer.DeleteResponse, error) {
	e := ""
	var mo, dr *string
	if req.State.MoCallBackURL != nil {
		mo = &e
	}
	if req.State.DrCallBackURL != nil {
		dr = &e
	}
	if mo == nil && dr == nil {
		return infer.DeleteResponse{}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if _, err := c.postAccountSettings(ctx, accountSettingsForm(mo, dr)); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("clear account settings: %w", err)
	}
	return infer.DeleteResponse{}, nil
}

// GetAccountSettings reads the default SMS webhooks and rate limits of the provider's account.
type GetAccountSettings struct{}

// GetAccountSettingsArgs has no arguments.
type GetAccountSettingsArgs struct{}

// GetAccountSettingsResult is the account settings.
type GetAccountSettingsResult struct {
	MoCallBackURL      string `pulumi:"moCallBackUrl"`
	DrCallBackURL      string `pulumi:"drCallBackUrl"`
	MaxOutboundRequest *int   `pulumi:"maxOutboundRequest,optional"`
	MaxInboundRequest  *int   `pulumi:"maxInboundRequest,optional"`
	MaxCallsPerSecond  *int   `pulumi:"maxCallsPerSecond,optional"`
}

func (a *GetAccountSettingsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.MoCallBackURL, "Default inbound SMS webhook URL, empty when none is set.")
	an.Describe(&a.DrCallBackURL, "Default SMS delivery receipt webhook URL, empty when none is set.")
	an.Describe(&a.MaxOutboundRequest, "Maximum outbound SMS requests per second.")
	an.Describe(&a.MaxInboundRequest, "Maximum inbound SMS requests per second.")
	an.Describe(&a.MaxCallsPerSecond, "Maximum API calls per second.")
}

func (f *GetAccountSettings) Annotate(a infer.Annotator) {
	a.Describe(&f, "Read the default SMS webhooks and rate limits of the provider's Vonage account. Vonage has no GET "+
		"for these, so this sends POST /account/settings with no fields, which returns the current values unchanged.")
}

func (f *GetAccountSettings) Invoke(ctx context.Context, _ infer.FunctionRequest[GetAccountSettingsArgs]) (infer.FunctionResponse[GetAccountSettingsResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetAccountSettingsResult]{}, err
	}
	w, err := c.postAccountSettings(ctx, nil)
	if err != nil {
		return infer.FunctionResponse[GetAccountSettingsResult]{}, fmt.Errorf("getAccountSettings: %w", err)
	}
	return infer.FunctionResponse[GetAccountSettingsResult]{Output: GetAccountSettingsResult{
		MoCallBackURL: w.MoCallBackURL, DrCallBackURL: w.DrCallBackURL,
		MaxOutboundRequest: w.MaxOutboundRequest.ptr(), MaxInboundRequest: w.MaxInboundRequest.ptr(),
		MaxCallsPerSecond: w.MaxCallsPerSecond.ptr(),
	}}, nil
}
