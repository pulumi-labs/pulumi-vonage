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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// External Accounts API (https://developer.vonage.com/en/api/external-accounts, spec v0.1.6,
// beta). The request and response shapes follow the published OpenAPI document and have not
// been exercised against a live Vonage account. The API links a Facebook Messenger, WhatsApp
// or Viber Service Message account that lives outside Vonage to a Vonage application.

const (
	extAccountsBase = "/beta/chatapp-accounts"
	// extPageSize is the page size requested when listing accounts.
	extPageSize = 100
	// maxExtPages bounds the pages a listing follows.
	maxExtPages = 100
)

// extChannels are the channels the API can read back one account of. The spec also lists
// rcs, but offers no read endpoint for it, so the provider cannot manage or look one up.
var extChannels = []string{"messenger", "viber_service_msg", "whatsapp"}

func validExtChannel(s string) bool { return containsString(extChannels, s) }

type wireExtAccount struct {
	Name         string   `json:"name,omitempty"`
	Applications []string `json:"applications,omitempty"`
	ExternalID   string   `json:"external_id"`
	APIKey       string   `json:"api_key,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	AccessToken  string   `json:"access_token,omitempty"`
}

type wireMessengerCreate struct {
	ExternalID  string `json:"external_id"`
	AccessToken string `json:"access_token"`
	Name        string `json:"name,omitempty"`
}

// wireMessengerPatch carries only the fields the provider manages; applications are owned by
// ExternalAccountApplicationLink and are never sent.
type wireMessengerPatch struct {
	Name        *string `json:"name,omitempty"`
	AccessToken *string `json:"access_token,omitempty"`
}

type wireExtAccountPage struct {
	Embedded []wireExtAccount `json:"_embedded"`
	Links    *struct {
		Next *struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links"`
}

func extAccountPath(channel, externalID string) string {
	return extAccountsBase + "/" + url.PathEscape(channel) + "/" + url.PathEscape(externalID)
}

// CreateMessengerAccount registers a Messenger page.
func (c *Client) CreateMessengerAccount(ctx context.Context, in wireMessengerCreate) (*wireExtAccount, error) {
	var out wireExtAccount
	if err := c.apiJSON(ctx, http.MethodPost, extAccountsBase+"/messenger", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetExtAccount returns nil, nil when the account does not exist.
func (c *Client) GetExtAccount(ctx context.Context, channel, externalID string) (*wireExtAccount, error) {
	var out wireExtAccount
	err := c.apiJSON(ctx, http.MethodGet, extAccountPath(channel, externalID), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PatchMessengerAccount updates a Messenger account.
func (c *Client) PatchMessengerAccount(ctx context.Context, externalID string, in wireMessengerPatch) error {
	return c.apiJSON(ctx, http.MethodPatch, extAccountPath("messenger", externalID), nil, in, nil)
}

// DeleteMessengerAccount deletes a Messenger account; a missing one is not an error.
func (c *Client) DeleteMessengerAccount(ctx context.Context, externalID string) error {
	err := c.apiJSON(ctx, http.MethodDelete, extAccountPath("messenger", externalID), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// ListExtAccounts returns every account, or those of one channel. It follows the
// next link, and refuses to guess when a full page arrives with no paging links at all.
func (c *Client) ListExtAccounts(ctx context.Context, channel string) ([]wireExtAccount, error) {
	out := []wireExtAccount{}
	for page := 1; page <= maxExtPages; page++ {
		q := url.Values{"page_number": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(extPageSize)}}
		if channel != "" {
			q.Set("provider", channel)
		}
		var pg wireExtAccountPage
		if err := c.apiJSON(ctx, http.MethodGet, extAccountsBase+"/", q, nil, &pg); err != nil {
			return nil, err
		}
		out = append(out, pg.Embedded...)
		if pg.Links != nil {
			if pg.Links.Next == nil || pg.Links.Next.Href == "" {
				return out, nil
			}
			continue
		}
		if len(pg.Embedded) < extPageSize {
			return out, nil
		}
		return nil, fmt.Errorf("vonage: GET %s/ page %d: full page with no _links, refusing to guess whether more pages exist", extAccountsBase, page)
	}
	return nil, fmt.Errorf("vonage: GET %s/: more than %d pages", extAccountsBase, maxExtPages)
}

// LinkExtApplication links an application to an account (one application per account).
func (c *Client) LinkExtApplication(ctx context.Context, channel, externalID, applicationID string) error {
	body := map[string]string{"application": applicationID}
	return c.apiJSON(ctx, http.MethodPost, extAccountPath(channel, externalID)+"/applications", nil, body, nil)
}

// UnlinkExtApplication removes the link. The API answers 409 when the application is not
// linked or does not exist, and 404 when the account is gone; both mean there is nothing to remove.
func (c *Client) UnlinkExtApplication(ctx context.Context, channel, externalID, applicationID string) error {
	err := c.apiJSON(ctx, http.MethodDelete, extAccountPath(channel, externalID)+"/applications/"+url.PathEscape(applicationID), nil, nil, nil)
	var ae *apiError
	if isNotFound(err) || (errors.As(err, &ae) && ae.Status == http.StatusConflict) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// MessengerAccount
// ---------------------------------------------------------------------------

// MessengerAccount registers a Facebook Messenger page with Vonage.
type MessengerAccount struct{}

// MessengerAccountArgs are the inputs of a MessengerAccount.
type MessengerAccountArgs struct {
	ExternalID  string  `pulumi:"externalId"`
	AccessToken string  `pulumi:"accessToken" provider:"secret"`
	Name        *string `pulumi:"name,optional"`
}

func (a *MessengerAccountArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.ExternalID, "The Facebook Page ID. Also the resource ID. Changing it replaces the account.")
	an.Describe(&a.AccessToken, "The Facebook Page access token. Secret. Changing it rotates the token in place.")
	an.Describe(&a.Name, "A custom name for the account. When unset, the name Vonage holds (if any) is left alone and is not a diff.")
}

// MessengerAccountState is the persisted state of a MessengerAccount.
type MessengerAccountState struct {
	MessengerAccountArgs

	// APIKey is the api_key Vonage reports for the account.
	APIKey string `pulumi:"apiKey"`
	// Applications lists the linked application IDs (Vonage allows one).
	Applications []string `pulumi:"applications"`
}

func (s *MessengerAccountState) Annotate(a infer.Annotator) {
	a.Describe(&s.APIKey, "The api_key Vonage reports for the account.")
	a.Describe(&s.Applications, "IDs of the applications linked to the account (at most one). Output only; link one with ExternalAccountApplicationLink.")
}

func (r *MessengerAccount) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Facebook Messenger page registered with Vonage (beta External Accounts API). Link an application with ExternalAccountApplicationLink. Shapes follow the published specification and were not verified against a live account.")
}

var _ infer.CustomResource[MessengerAccountArgs, MessengerAccountState] = (*MessengerAccount)(nil)
var _ infer.CustomCheck[MessengerAccountArgs] = (*MessengerAccount)(nil)
var _ infer.CustomRead[MessengerAccountArgs, MessengerAccountState] = (*MessengerAccount)(nil)
var _ infer.CustomUpdate[MessengerAccountArgs, MessengerAccountState] = (*MessengerAccount)(nil)
var _ infer.CustomDiff[MessengerAccountArgs, MessengerAccountState] = (*MessengerAccount)(nil)
var _ infer.CustomDelete[MessengerAccountState] = (*MessengerAccount)(nil)

func extIDFailure(prop, id string) []p.CheckFailure {
	if strings.TrimSpace(id) == "" || strings.Contains(id, "/") {
		return []p.CheckFailure{{Property: prop, Reason: prop + " must be non-empty and must not contain \"/\""}}
	}
	return nil
}

func (r *MessengerAccount) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[MessengerAccountArgs], error) {
	args, failures, err := infer.DefaultCheck[MessengerAccountArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[MessengerAccountArgs]{Inputs: args, Failures: failures}, err
	}
	failures = append(failures, extIDFailure("externalId", args.ExternalID)...)
	if strings.TrimSpace(args.AccessToken) == "" {
		failures = append(failures, p.CheckFailure{Property: "accessToken", Reason: "accessToken must not be empty"})
	}
	return infer.CheckResponse[MessengerAccountArgs]{Inputs: args, Failures: dropUnknownFailures(failures, req.NewInputs)}, nil
}

// keepName reports the server name as an input: unset stays unset while the server has none.
func keepName(in *string, server string) *string {
	if in != nil || server != "" {
		v := server
		return &v
	}
	return nil
}

func derefOr(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}

func messengerStateFrom(w *wireExtAccount, externalID string, prior MessengerAccountArgs) MessengerAccountState {
	id := w.ExternalID
	if id == "" {
		id = externalID
	}
	token := w.AccessToken
	if token == "" {
		token = prior.AccessToken
	}
	apps := w.Applications
	if apps == nil {
		apps = []string{}
	}
	return MessengerAccountState{
		MessengerAccountArgs: MessengerAccountArgs{ExternalID: id, AccessToken: token, Name: keepName(prior.Name, w.Name)},
		APIKey:               w.APIKey,
		Applications:         apps,
	}
}

func (r *MessengerAccount) Create(ctx context.Context, req infer.CreateRequest[MessengerAccountArgs]) (infer.CreateResponse[MessengerAccountState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[MessengerAccountState]{Output: MessengerAccountState{MessengerAccountArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[MessengerAccountState]{}, err
	}
	w, err := c.CreateMessengerAccount(ctx, wireMessengerCreate{ExternalID: a.ExternalID, AccessToken: a.AccessToken, Name: derefOr(a.Name, "")})
	if err != nil {
		return infer.CreateResponse[MessengerAccountState]{}, fmt.Errorf("create messenger account %q: %w", a.ExternalID, err)
	}
	return infer.CreateResponse[MessengerAccountState]{ID: a.ExternalID, Output: messengerStateFrom(w, a.ExternalID, a)}, nil
}

func (r *MessengerAccount) Read(ctx context.Context, req infer.ReadRequest[MessengerAccountArgs, MessengerAccountState]) (infer.ReadResponse[MessengerAccountArgs, MessengerAccountState], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.ReadResponse[MessengerAccountArgs, MessengerAccountState]{}, err
	}
	w, err := c.GetExtAccount(ctx, "messenger", req.ID)
	if err != nil {
		return infer.ReadResponse[MessengerAccountArgs, MessengerAccountState]{}, fmt.Errorf("read messenger account %s: %w", req.ID, err)
	}
	if w == nil {
		if err := notFoundOnImport("messenger account", req.ID, req.Inputs, req.State); err != nil {
			return infer.ReadResponse[MessengerAccountArgs, MessengerAccountState]{}, err
		}
		return infer.ReadResponse[MessengerAccountArgs, MessengerAccountState]{}, nil
	}
	st := messengerStateFrom(w, req.ID, req.Inputs)
	return infer.ReadResponse[MessengerAccountArgs, MessengerAccountState]{ID: req.ID, Inputs: st.MessengerAccountArgs, State: st}, nil
}

func (r *MessengerAccount) Diff(ctx context.Context, req infer.DiffRequest[MessengerAccountArgs, MessengerAccountState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.MessengerAccountArgs
	if in.ExternalID != old.ExternalID {
		d["externalId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.AccessToken != old.AccessToken {
		d["accessToken"] = p.PropertyDiff{Kind: p.Update}
	}
	if in.Name != nil && *in.Name != derefOr(old.Name, "") {
		d["name"] = p.PropertyDiff{Kind: p.Update}
	}
	// One account per page ID: the old registration must go before the new one is made.
	return infer.DiffResponse{HasChanges: len(d) > 0, DeleteBeforeReplace: true, DetailedDiff: d}, nil
}

func (r *MessengerAccount) Update(ctx context.Context, req infer.UpdateRequest[MessengerAccountArgs, MessengerAccountState]) (infer.UpdateResponse[MessengerAccountState], error) {
	a := req.Inputs
	st := req.State
	st.MessengerAccountArgs = a
	if req.DryRun {
		return infer.UpdateResponse[MessengerAccountState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[MessengerAccountState]{}, err
	}
	// An unset name is not managed, so it is not sent; the token is always sent.
	token := a.AccessToken
	if err := c.PatchMessengerAccount(ctx, req.ID, wireMessengerPatch{Name: a.Name, AccessToken: &token}); err != nil {
		return infer.UpdateResponse[MessengerAccountState]{}, fmt.Errorf("update messenger account %s: %w", req.ID, err)
	}
	return infer.UpdateResponse[MessengerAccountState]{Output: st}, nil
}

func (r *MessengerAccount) Delete(ctx context.Context, req infer.DeleteRequest[MessengerAccountState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.DeleteMessengerAccount(ctx, req.ID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete messenger account %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// ExternalAccountApplicationLink
// ---------------------------------------------------------------------------

// ExternalAccountApplicationLink links a Vonage application to an external account.
type ExternalAccountApplicationLink struct{}

// ExternalAccountApplicationLinkArgs are the inputs of an ExternalAccountApplicationLink.
type ExternalAccountApplicationLinkArgs struct {
	Channel       string `pulumi:"channel"`
	ExternalID    string `pulumi:"externalId"`
	ApplicationID string `pulumi:"applicationId"`
}

func (a *ExternalAccountApplicationLinkArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Channel, "The account's channel: messenger, viber_service_msg or whatsapp. Changing it replaces the link.")
	an.Describe(&a.ExternalID, "The account's external ID: the Facebook Page ID, the Viber Service Message ID or the WhatsApp number. Changing it replaces the link.")
	an.Describe(&a.ApplicationID, "ID of a Vonage application with the messages capability. An account has at most one. Changing it replaces the link.")
}

// ExternalAccountApplicationLinkState is the persisted state of an ExternalAccountApplicationLink.
type ExternalAccountApplicationLinkState struct {
	ExternalAccountApplicationLinkArgs
}

func (r *ExternalAccountApplicationLink) Annotate(a infer.Annotator) {
	a.Describe(&r, "Links a Vonage application to a Messenger, Viber Service Message or WhatsApp account (one application per account). The ID is channel/externalId/applicationId. Removing the resource unlinks the application and never deletes the account or the application. Shapes follow the published specification and were not verified against a live account.")
}

var _ infer.CustomResource[ExternalAccountApplicationLinkArgs, ExternalAccountApplicationLinkState] = (*ExternalAccountApplicationLink)(nil)
var _ infer.CustomCheck[ExternalAccountApplicationLinkArgs] = (*ExternalAccountApplicationLink)(nil)
var _ infer.CustomRead[ExternalAccountApplicationLinkArgs, ExternalAccountApplicationLinkState] = (*ExternalAccountApplicationLink)(nil)
var _ infer.CustomDiff[ExternalAccountApplicationLinkArgs, ExternalAccountApplicationLinkState] = (*ExternalAccountApplicationLink)(nil)
var _ infer.CustomDelete[ExternalAccountApplicationLinkState] = (*ExternalAccountApplicationLink)(nil)

func extLinkID(channel, externalID, applicationID string) string {
	return channel + "/" + externalID + "/" + applicationID
}

func (r *ExternalAccountApplicationLink) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[ExternalAccountApplicationLinkArgs], error) {
	args, failures, err := infer.DefaultCheck[ExternalAccountApplicationLinkArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[ExternalAccountApplicationLinkArgs]{Inputs: args, Failures: failures}, err
	}
	if !validExtChannel(args.Channel) {
		failures = append(failures, p.CheckFailure{Property: "channel", Reason: "channel must be one of " + strings.Join(extChannels, ", ")})
	}
	failures = append(failures, extIDFailure("externalId", args.ExternalID)...)
	failures = append(failures, extIDFailure("applicationId", args.ApplicationID)...)
	return infer.CheckResponse[ExternalAccountApplicationLinkArgs]{Inputs: args, Failures: dropUnknownFailures(failures, req.NewInputs)}, nil
}

func (r *ExternalAccountApplicationLink) Create(ctx context.Context, req infer.CreateRequest[ExternalAccountApplicationLinkArgs]) (infer.CreateResponse[ExternalAccountApplicationLinkState], error) {
	a := req.Inputs
	st := ExternalAccountApplicationLinkState{ExternalAccountApplicationLinkArgs: a}
	if req.DryRun {
		return infer.CreateResponse[ExternalAccountApplicationLinkState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[ExternalAccountApplicationLinkState]{}, err
	}
	id := extLinkID(a.Channel, a.ExternalID, a.ApplicationID)
	acct, err := c.GetExtAccount(ctx, a.Channel, a.ExternalID)
	if err != nil {
		return infer.CreateResponse[ExternalAccountApplicationLinkState]{}, fmt.Errorf("read %s account %q: %w", a.Channel, a.ExternalID, err)
	}
	if acct == nil {
		return infer.CreateResponse[ExternalAccountApplicationLinkState]{}, fmt.Errorf("%s account %q does not exist", a.Channel, a.ExternalID)
	}
	// Already linked (for example a retry after a lost response): adopt it rather than hit the one-application limit.
	if !containsString(acct.Applications, a.ApplicationID) {
		if err := c.LinkExtApplication(ctx, a.Channel, a.ExternalID, a.ApplicationID); err != nil {
			return infer.CreateResponse[ExternalAccountApplicationLinkState]{}, fmt.Errorf("link application %s to %s account %q: %w", a.ApplicationID, a.Channel, a.ExternalID, err)
		}
	}
	return infer.CreateResponse[ExternalAccountApplicationLinkState]{ID: id, Output: st}, nil
}

func (r *ExternalAccountApplicationLink) Read(ctx context.Context, req infer.ReadRequest[ExternalAccountApplicationLinkArgs, ExternalAccountApplicationLinkState]) (infer.ReadResponse[ExternalAccountApplicationLinkArgs, ExternalAccountApplicationLinkState], error) {
	type resp = infer.ReadResponse[ExternalAccountApplicationLinkArgs, ExternalAccountApplicationLinkState]
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" || !validExtChannel(parts[0]) {
		return resp{}, fmt.Errorf("invalid ExternalAccountApplicationLink ID %q: expected channel/externalId/applicationId with channel one of %s", req.ID, strings.Join(extChannels, ", "))
	}
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	acct, err := c.GetExtAccount(ctx, parts[0], parts[1])
	if err != nil {
		return resp{}, fmt.Errorf("read %s account %s: %w", parts[0], parts[1], err)
	}
	if acct == nil || !containsString(acct.Applications, parts[2]) {
		if err := notFoundOnImport("external account application link", req.ID, req.Inputs, req.State); err != nil {
			return resp{}, err
		}
		return resp{}, nil
	}
	args := ExternalAccountApplicationLinkArgs{Channel: parts[0], ExternalID: parts[1], ApplicationID: parts[2]}
	return resp{ID: req.ID, Inputs: args, State: ExternalAccountApplicationLinkState{ExternalAccountApplicationLinkArgs: args}}, nil
}

func (r *ExternalAccountApplicationLink) Diff(ctx context.Context, req infer.DiffRequest[ExternalAccountApplicationLinkArgs, ExternalAccountApplicationLinkState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.ExternalAccountApplicationLinkArgs
	if in.Channel != old.Channel {
		d["channel"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.ExternalID != old.ExternalID {
		d["externalId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.ApplicationID != old.ApplicationID {
		d["applicationId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	// An account holds one application, so the old link must go before a new one is made.
	return infer.DiffResponse{HasChanges: len(d) > 0, DeleteBeforeReplace: true, DetailedDiff: d}, nil
}

func (r *ExternalAccountApplicationLink) Delete(ctx context.Context, req infer.DeleteRequest[ExternalAccountApplicationLinkState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	s := req.State
	if err := c.UnlinkExtApplication(ctx, s.Channel, s.ExternalID, s.ApplicationID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("unlink application %s from %s account %q: %w", s.ApplicationID, s.Channel, s.ExternalID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// Lookups
// ---------------------------------------------------------------------------

// GetExternalAccount looks up one external account.
type GetExternalAccount struct{}

// GetExternalAccountArgs are the lookup arguments.
type GetExternalAccountArgs struct {
	Channel    string `pulumi:"channel"`
	ExternalID string `pulumi:"externalId"`
}

func (a *GetExternalAccountArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Channel, "The account's channel: messenger, viber_service_msg or whatsapp.")
	an.Describe(&a.ExternalID, "The account's external ID: the Facebook Page ID, the Viber Service Message ID or the WhatsApp number.")
}

// GetExternalAccountResult is an external account.
type GetExternalAccountResult struct {
	Channel      string   `pulumi:"channel"`
	ExternalID   string   `pulumi:"externalId"`
	APIKey       string   `pulumi:"apiKey"`
	Name         string   `pulumi:"name"`
	Applications []string `pulumi:"applications"`
	// AccessToken is set only for a Messenger account.
	AccessToken *string `pulumi:"accessToken,optional" provider:"secret"`
}

func (a *GetExternalAccountResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Channel, "The account's channel.")
	an.Describe(&a.ExternalID, "The account's external ID.")
	an.Describe(&a.APIKey, "The api_key Vonage reports for the account.")
	an.Describe(&a.Name, "The account name (empty when none is set).")
	an.Describe(&a.Applications, "IDs of the linked applications (at most one).")
	an.Describe(&a.AccessToken, "The Facebook Page access token. Secret. Set only for a Messenger account.")
}

func (f *GetExternalAccount) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Messenger, Viber Service Message or WhatsApp account registered with Vonage by channel and external ID. Fails if it does not exist.")
}

func (f *GetExternalAccount) Invoke(ctx context.Context, req infer.FunctionRequest[GetExternalAccountArgs]) (infer.FunctionResponse[GetExternalAccountResult], error) {
	in := req.Input
	if !validExtChannel(in.Channel) || in.ExternalID == "" {
		return infer.FunctionResponse[GetExternalAccountResult]{}, fmt.Errorf("getExternalAccount: externalId and a channel (%s) are required", strings.Join(extChannels, ", "))
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetExternalAccountResult]{}, err
	}
	w, err := c.GetExtAccount(ctx, in.Channel, in.ExternalID)
	if err != nil {
		return infer.FunctionResponse[GetExternalAccountResult]{}, fmt.Errorf("getExternalAccount: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[GetExternalAccountResult]{}, fmt.Errorf("getExternalAccount: %s account %q was not found", in.Channel, in.ExternalID)
	}
	return infer.FunctionResponse[GetExternalAccountResult]{Output: extAccountResult(in.Channel, in.ExternalID, w)}, nil
}

func extAccountResult(channel, externalID string, w *wireExtAccount) GetExternalAccountResult {
	r := GetExternalAccountResult{Channel: channel, ExternalID: externalID, APIKey: w.APIKey, Name: w.Name, Applications: w.Applications}
	if r.Applications == nil {
		r.Applications = []string{}
	}
	if w.Provider != "" {
		r.Channel = w.Provider
	}
	if w.ExternalID != "" {
		r.ExternalID = w.ExternalID
	}
	if w.AccessToken != "" {
		t := w.AccessToken
		r.AccessToken = &t
	}
	return r
}

// GetMessengerAccount looks up one Messenger account.
type GetMessengerAccount struct{}

// GetMessengerAccountArgs are the lookup arguments.
type GetMessengerAccountArgs struct {
	ExternalID string `pulumi:"externalId"`
}

func (a *GetMessengerAccountArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.ExternalID, "The Facebook Page ID.")
}

func (f *GetMessengerAccount) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Messenger account registered with Vonage by Facebook Page ID, including its page access token (secret). Fails if it does not exist.")
}

func (f *GetMessengerAccount) Invoke(ctx context.Context, req infer.FunctionRequest[GetMessengerAccountArgs]) (infer.FunctionResponse[GetExternalAccountResult], error) {
	if req.Input.ExternalID == "" {
		return infer.FunctionResponse[GetExternalAccountResult]{}, fmt.Errorf("getMessengerAccount: externalId is required")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetExternalAccountResult]{}, err
	}
	w, err := c.GetExtAccount(ctx, "messenger", req.Input.ExternalID)
	if err != nil {
		return infer.FunctionResponse[GetExternalAccountResult]{}, fmt.Errorf("getMessengerAccount: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[GetExternalAccountResult]{}, fmt.Errorf("getMessengerAccount: messenger account %q was not found", req.Input.ExternalID)
	}
	return infer.FunctionResponse[GetExternalAccountResult]{Output: extAccountResult("messenger", req.Input.ExternalID, w)}, nil
}

// GetExternalAccounts lists the external accounts.
type GetExternalAccounts struct{}

// GetExternalAccountsArgs are the lookup arguments.
type GetExternalAccountsArgs struct {
	Channel *string `pulumi:"channel,optional"`
}

func (a *GetExternalAccountsArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Channel, "Only list accounts of this channel: messenger, viber_service_msg, whatsapp or rcs. Defaults to every channel.")
}

// ExternalAccountSummary is one listed account.
type ExternalAccountSummary struct {
	Channel      string   `pulumi:"channel"`
	ExternalID   string   `pulumi:"externalId"`
	APIKey       string   `pulumi:"apiKey"`
	Name         string   `pulumi:"name"`
	Applications []string `pulumi:"applications"`
}

func (a *ExternalAccountSummary) Annotate(an infer.Annotator) {
	an.Describe(&a.Channel, "The account's channel.")
	an.Describe(&a.ExternalID, "The account's external ID.")
	an.Describe(&a.APIKey, "The api_key Vonage reports for the account.")
	an.Describe(&a.Name, "The account name (empty when none is set).")
	an.Describe(&a.Applications, "IDs of the linked applications (at most one). The page access token is never listed; use getMessengerAccount.")
}

// GetExternalAccountsResult lists external accounts.
type GetExternalAccountsResult struct {
	Accounts []ExternalAccountSummary `pulumi:"accounts"`
}

func (a *GetExternalAccountsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Accounts, "Every matching account, across all pages.")
}

func (f *GetExternalAccounts) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the external (Messenger, Viber Service Message, WhatsApp, RCS) accounts registered with Vonage, following every page.")
}

func (f *GetExternalAccounts) Invoke(ctx context.Context, req infer.FunctionRequest[GetExternalAccountsArgs]) (infer.FunctionResponse[GetExternalAccountsResult], error) {
	channel := derefOr(req.Input.Channel, "")
	if channel != "" && channel != "rcs" && !validExtChannel(channel) {
		return infer.FunctionResponse[GetExternalAccountsResult]{}, fmt.Errorf("getExternalAccounts: channel must be one of messenger, viber_service_msg, whatsapp, rcs")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetExternalAccountsResult]{}, err
	}
	all, err := c.ListExtAccounts(ctx, channel)
	if err != nil {
		return infer.FunctionResponse[GetExternalAccountsResult]{}, fmt.Errorf("getExternalAccounts: %w", err)
	}
	out := make([]ExternalAccountSummary, 0, len(all))
	for _, w := range all {
		apps := w.Applications
		if apps == nil {
			apps = []string{}
		}
		out = append(out, ExternalAccountSummary{Channel: w.Provider, ExternalID: w.ExternalID, APIKey: w.APIKey, Name: w.Name, Applications: apps})
	}
	return infer.FunctionResponse[GetExternalAccountsResult]{Output: GetExternalAccountsResult{Accounts: out}}, nil
}

// GetExternalAccountApplicationLink reports whether an application is linked to an external account.
type GetExternalAccountApplicationLink struct{}

// GetExternalAccountApplicationLinkArgs are the lookup arguments.
type GetExternalAccountApplicationLinkArgs struct {
	Channel       string `pulumi:"channel"`
	ExternalID    string `pulumi:"externalId"`
	ApplicationID string `pulumi:"applicationId"`
}

func (a *GetExternalAccountApplicationLinkArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Channel, "The account's channel: messenger, viber_service_msg or whatsapp.")
	an.Describe(&a.ExternalID, "The account's external ID.")
	an.Describe(&a.ApplicationID, "The application ID to look for.")
}

// GetExternalAccountApplicationLinkResult says whether the link exists.
type GetExternalAccountApplicationLinkResult struct {
	Channel       string `pulumi:"channel"`
	ExternalID    string `pulumi:"externalId"`
	ApplicationID string `pulumi:"applicationId"`
	Linked        bool   `pulumi:"linked"`
}

func (a *GetExternalAccountApplicationLinkResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Channel, "The account's channel.")
	an.Describe(&a.ExternalID, "The account's external ID.")
	an.Describe(&a.ApplicationID, "The application ID that was looked for.")
	an.Describe(&a.Linked, "Whether the application is linked to the account.")
}

func (f *GetExternalAccountApplicationLink) Annotate(a infer.Annotator) {
	a.Describe(&f, "Check whether an application is linked to an external account. Fails if the account does not exist.")
}

func (f *GetExternalAccountApplicationLink) Invoke(ctx context.Context, req infer.FunctionRequest[GetExternalAccountApplicationLinkArgs]) (infer.FunctionResponse[GetExternalAccountApplicationLinkResult], error) {
	in := req.Input
	if !validExtChannel(in.Channel) || in.ExternalID == "" || in.ApplicationID == "" {
		return infer.FunctionResponse[GetExternalAccountApplicationLinkResult]{}, fmt.Errorf("getExternalAccountApplicationLink: externalId, applicationId and a channel (%s) are required", strings.Join(extChannels, ", "))
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetExternalAccountApplicationLinkResult]{}, err
	}
	w, err := c.GetExtAccount(ctx, in.Channel, in.ExternalID)
	if err != nil {
		return infer.FunctionResponse[GetExternalAccountApplicationLinkResult]{}, fmt.Errorf("getExternalAccountApplicationLink: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[GetExternalAccountApplicationLinkResult]{}, fmt.Errorf("getExternalAccountApplicationLink: %s account %q was not found", in.Channel, in.ExternalID)
	}
	return infer.FunctionResponse[GetExternalAccountApplicationLinkResult]{Output: GetExternalAccountApplicationLinkResult{
		Channel: in.Channel, ExternalID: in.ExternalID, ApplicationID: in.ApplicationID, Linked: containsString(w.Applications, in.ApplicationID)}}, nil
}
