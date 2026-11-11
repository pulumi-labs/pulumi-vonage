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
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// Network Registry application profiles and their shares (Network Registry OpenAPI v1.0.0, tag
// "Application Profile"). Shapes follow the published document and have not been exercised against a
// live Vonage account.

// ---------------------------------------------------------------------------
// Wire types and client
// ---------------------------------------------------------------------------

type wireNRNetworkAPI struct {
	NetworkAPIID string   `json:"network_api_id"`
	PurposeIDs   []string `json:"purpose_ids"`
}

// wireNRAppProfileIn is the writable part of an application profile (the create body).
type wireNRAppProfileIn struct {
	BusinessProfileID          string             `json:"business_profile_id"`
	Name                       string             `json:"name"`
	Description                string             `json:"description,omitempty"`
	CategoryID                 string             `json:"category_id"`
	NetworkAPIs                []wireNRNetworkAPI `json:"network_apis"`
	TermsAndConditionsAccepted *bool              `json:"terms_and_conditions_accepted,omitempty"`
	InfrastructureLocations    []string           `json:"infrastructure_locations"`
	PrivacyPolicyURL           string             `json:"privacy_policy_url,omitempty"`
}

type wireNRAppProfile struct {
	wireNRAppProfileIn
	ID        string       `json:"id"`
	AccountID string       `json:"account_id"`
	Status    wireNRStatus `json:"status"`
	CreatedAt string       `json:"created_at"`
	UpdatedAt string       `json:"updated_at"`
	DeleteAt  string       `json:"delete_at"`
}

func nrAppProfilePath(id string) string {
	return nrBase + "/application-profiles/" + url.PathEscape(id)
}

// CreateNRAppProfile registers an application profile. The call is never replayed after an unknown
// outcome (POST), so a retry cannot register the profile twice.
func (c *Client) CreateNRAppProfile(ctx context.Context, in wireNRAppProfileIn) (*wireNRAppProfile, error) {
	var out wireNRAppProfile
	if err := c.apiJSON(ctx, http.MethodPost, nrBase+"/application-profiles", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetNRAppProfile returns nil, nil when the profile does not exist or has been deleted (Vonage
// deletes softly and reports delete_at).
func (c *Client) GetNRAppProfile(ctx context.Context, id string) (*wireNRAppProfile, error) {
	var out wireNRAppProfile
	err := c.apiJSON(ctx, http.MethodGet, nrAppProfilePath(id), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if out.DeleteAt != "" {
		return nil, nil
	}
	return &out, nil
}

// DeleteNRAppProfile deletes a profile; a missing profile is not an error.
func (c *Client) DeleteNRAppProfile(ctx context.Context, id string) error {
	err := c.apiJSON(ctx, http.MethodDelete, nrAppProfilePath(id), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// ListNRAppProfiles returns the application profiles visible to the account, skipping soft-deleted
// ones. Empty filters are not sent.
func (c *Client) ListNRAppProfiles(ctx context.Context, businessProfileID, status string, shared bool) ([]wireNRAppProfile, error) {
	q := url.Values{"limit": {fmt.Sprint(nrPageLimit)}}
	if businessProfileID != "" {
		q.Set("business_profile_id", businessProfileID)
	}
	if status != "" {
		q.Set("status", status)
	}
	if shared {
		q.Set("shared", "true")
	}
	all, err := nrList[wireNRAppProfile](ctx, c, nrBase+"/application-profiles", q, "application_profiles")
	if err != nil {
		return nil, err
	}
	out := []wireNRAppProfile{}
	for _, w := range all {
		if w.DeleteAt == "" {
			out = append(out, w)
		}
	}
	return out, nil
}

// ListNRAppProfileShares returns the accounts the profile is shared with. A missing profile is
// reported as a 404 apiError.
func (c *Client) ListNRAppProfileShares(ctx context.Context, id string) ([]string, error) {
	type item struct {
		AccountID string `json:"account_id"`
	}
	items, err := nrList[item](ctx, c, nrAppProfilePath(id)+"/shared", url.Values{"limit": {fmt.Sprint(nrPageLimit)}}, "account_ids")
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, it := range items {
		out = append(out, it.AccountID)
	}
	return out, nil
}

func (c *Client) ShareNRAppProfile(ctx context.Context, id, accountID string) error {
	return c.apiJSON(ctx, http.MethodPost, nrAppProfilePath(id)+"/shared", nil, map[string]string{"account_id": accountID}, nil)
}

// UnshareNRAppProfile stops sharing; an absent share or profile is not an error.
func (c *Client) UnshareNRAppProfile(ctx context.Context, id, accountID string) error {
	err := c.apiJSON(ctx, http.MethodDelete, nrAppProfilePath(id)+"/shared/"+url.PathEscape(accountID), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// NetworkApplicationProfile
// ---------------------------------------------------------------------------

// NetworkApplicationProfile registers an application with the Vonage Network Registry: which Network
// APIs it uses and why, so the mobile operators can approve it.
type NetworkApplicationProfile struct{}

// NetworkApplicationProfileApi is one Network API an application profile asks to use.
type NetworkApplicationProfileApi struct {
	NetworkApiID string   `pulumi:"networkApiId"`
	PurposeIDs   []string `pulumi:"purposeIds"`
}

func (a *NetworkApplicationProfileApi) Annotate(an infer.Annotator) {
	an.Describe(&a.NetworkApiID, "ID of the Network API, from getNetworkRegistryCatalogue networkApis (for example sim-swap).")
	an.Describe(&a.PurposeIDs, "IDs of the purposes the operator needs to validate the request, from getNetworkRegistryCatalogue purposes.")
}

// NetworkApplicationProfileArgs are the inputs of a NetworkApplicationProfile. The API has no update
// call, so changing any of them replaces the profile.
type NetworkApplicationProfileArgs struct {
	BusinessProfileID          string                         `pulumi:"businessProfileId"`
	Name                       string                         `pulumi:"name"`
	Description                *string                        `pulumi:"description,optional"`
	CategoryID                 string                         `pulumi:"categoryId"`
	NetworkApis                []NetworkApplicationProfileApi `pulumi:"networkApis"`
	InfrastructureLocations    []string                       `pulumi:"infrastructureLocations"`
	PrivacyPolicyURL           *string                        `pulumi:"privacyPolicyUrl,optional"`
	TermsAndConditionsAccepted *bool                          `pulumi:"termsAndConditionsAccepted,optional"`
}

func (a *NetworkApplicationProfileArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BusinessProfileID, "ID of the NetworkBusinessProfile that owns this application profile. Changing it replaces the profile.")
	an.Describe(&a.Name, "Name of the application (at most 100 characters). Changing it replaces the profile.")
	an.Describe(&a.Description, "Description of the application's use cases (at most 1000 characters). Optional fields left unset are not managed. Changing it replaces the profile.")
	an.Describe(&a.CategoryID, "Category of the application, from getNetworkRegistryCatalogue categories (for example finance). Changing it replaces the profile.")
	an.Describe(&a.NetworkApis, "The Network APIs the application uses and the purposes it states for each. Changing them replaces the profile.")
	an.Describe(&a.InfrastructureLocations, "Two-letter ISO 3166 codes of the countries the application's infrastructure runs in. Changing them replaces the profile.")
	an.Describe(&a.PrivacyPolicyURL, "URL of the application's privacy policy (at most 500 characters). Changing it replaces the profile.")
	an.Describe(&a.TermsAndConditionsAccepted, "Whether you accept the Network Registry terms and conditions (see getNetworkTermsAndConditions). Sent only when set. Changing it replaces the profile.")
}

// NetworkApplicationProfileState is the persisted state of a NetworkApplicationProfile.
type NetworkApplicationProfileState struct {
	NetworkApplicationProfileArgs

	AccountID string `pulumi:"accountId"`
	// Status is the overall approval status, which moves on its own as operators review the profile.
	Status        string  `pulumi:"status"`
	StatusMessage *string `pulumi:"statusMessage,optional"`
	CreatedAt     string  `pulumi:"createdAt"`
	UpdatedAt     string  `pulumi:"updatedAt"`
}

func (s *NetworkApplicationProfileState) Annotate(a infer.Annotator) {
	a.Describe(&s.AccountID, "ID of the Vonage account that owns the profile.")
	a.Describe(&s.Status, "Overall approval status: pending, provisioning, accepted, partially-accepted, rejected or failed. It changes as operators review the profile.")
	a.Describe(&s.StatusMessage, "Vonage's note about why the profile was rejected, when there is one.")
	a.Describe(&s.CreatedAt, "When the profile was created (RFC 3339).")
	a.Describe(&s.UpdatedAt, "When the profile was last updated (RFC 3339).")
}

func (r *NetworkApplicationProfile) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Vonage Network Registry application profile. Registering one is a submission that mobile operators review, so its status moves from pending towards accepted on its own. The API has no update call, so changing any input replaces the profile: the new one is created first, then the old one is deleted (Vonage deletes softly), and each account may hold at most 50 profiles. Shapes follow the published Network Registry OpenAPI definition and were not verified against a live account.")
}

var _ infer.CustomResource[NetworkApplicationProfileArgs, NetworkApplicationProfileState] = (*NetworkApplicationProfile)(nil)
var _ infer.CustomCheck[NetworkApplicationProfileArgs] = (*NetworkApplicationProfile)(nil)
var _ infer.CustomRead[NetworkApplicationProfileArgs, NetworkApplicationProfileState] = (*NetworkApplicationProfile)(nil)
var _ infer.CustomDiff[NetworkApplicationProfileArgs, NetworkApplicationProfileState] = (*NetworkApplicationProfile)(nil)
var _ infer.CustomDelete[NetworkApplicationProfileState] = (*NetworkApplicationProfile)(nil)

func (r *NetworkApplicationProfile) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[NetworkApplicationProfileArgs], error) {
	args, failures, err := infer.DefaultCheck[NetworkApplicationProfileArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[NetworkApplicationProfileArgs]{Inputs: args, Failures: failures}, err
	}
	fail := func(prop, reason string) { failures = append(failures, p.CheckFailure{Property: prop, Reason: reason}) }
	for prop, v := range map[string]string{"businessProfileId": args.BusinessProfileID, "name": args.Name, "categoryId": args.CategoryID} {
		if strings.TrimSpace(v) == "" {
			fail(prop, prop+" must not be empty")
		}
	}
	if len(args.Name) > 100 {
		fail("name", "name must be at most 100 characters")
	}
	if args.Description != nil && len(*args.Description) > 1000 {
		fail("description", "description must be at most 1000 characters")
	}
	if args.PrivacyPolicyURL != nil && len(*args.PrivacyPolicyURL) > 500 {
		fail("privacyPolicyUrl", "privacyPolicyUrl must be at most 500 characters")
	}
	if len(args.NetworkApis) == 0 {
		fail("networkApis", "networkApis must list at least one Network API")
	}
	for _, na := range args.NetworkApis {
		if strings.TrimSpace(na.NetworkApiID) == "" {
			fail("networkApis", "every networkApis entry needs a networkApiId")
		}
	}
	for _, c := range args.InfrastructureLocations {
		if !isCountryCode(c) {
			fail("infrastructureLocations", fmt.Sprintf("%q is not a two-letter ISO 3166 country code", c))
		}
	}
	return infer.CheckResponse[NetworkApplicationProfileArgs]{Inputs: args, Failures: dropUnknownFailures(failures, req.NewInputs)}, nil
}

func nrAppBody(a NetworkApplicationProfileArgs) wireNRAppProfileIn {
	apis := make([]wireNRNetworkAPI, 0, len(a.NetworkApis))
	for _, na := range a.NetworkApis {
		ids := na.PurposeIDs
		if ids == nil {
			ids = []string{}
		}
		apis = append(apis, wireNRNetworkAPI{NetworkAPIID: na.NetworkApiID, PurposeIDs: ids})
	}
	locs := a.InfrastructureLocations
	if locs == nil {
		locs = []string{}
	}
	return wireNRAppProfileIn{
		BusinessProfileID: a.BusinessProfileID, Name: a.Name, Description: deref(a.Description), CategoryID: a.CategoryID,
		NetworkAPIs: apis, TermsAndConditionsAccepted: a.TermsAndConditionsAccepted, InfrastructureLocations: locs,
		PrivacyPolicyURL: deref(a.PrivacyPolicyURL),
	}
}

// sameNetworkApis compares the lists as sets keyed by Network API, ignoring the order of the APIs and of
// the purposes.
func sameNetworkApis(a, b []NetworkApplicationProfileApi) bool {
	norm := func(in []NetworkApplicationProfileApi) []string {
		out := make([]string, 0, len(in))
		for _, na := range in {
			ids := slices.Clone(na.PurposeIDs)
			sort.Strings(ids)
			out = append(out, na.NetworkApiID+"="+strings.Join(ids, ","))
		}
		sort.Strings(out)
		return out
	}
	return slices.Equal(norm(a), norm(b))
}

func nrAppArgsFrom(w *wireNRAppProfile, prior NetworkApplicationProfileArgs, importing bool) NetworkApplicationProfileArgs {
	apis := make([]NetworkApplicationProfileApi, 0, len(w.NetworkAPIs))
	for _, na := range w.NetworkAPIs {
		ids := na.PurposeIDs
		if ids == nil {
			ids = []string{}
		}
		apis = append(apis, NetworkApplicationProfileApi{NetworkApiID: na.NetworkAPIID, PurposeIDs: ids})
	}
	if sameNetworkApis(apis, prior.NetworkApis) {
		apis = prior.NetworkApis
	}
	locs := w.InfrastructureLocations
	if locs == nil {
		locs = []string{}
	}
	if sameCountries(locs, prior.InfrastructureLocations) {
		locs = prior.InfrastructureLocations
	}
	return NetworkApplicationProfileArgs{
		BusinessProfileID: w.BusinessProfileID, Name: w.Name, Description: adoptStr(prior.Description, w.Description, importing),
		CategoryID: w.CategoryID, NetworkApis: apis, InfrastructureLocations: locs,
		PrivacyPolicyURL:           adoptStr(prior.PrivacyPolicyURL, w.PrivacyPolicyURL, importing),
		TermsAndConditionsAccepted: adoptBool(prior.TermsAndConditionsAccepted, w.TermsAndConditionsAccepted, importing),
	}
}

func nrAppStateFrom(w *wireNRAppProfile, args NetworkApplicationProfileArgs) NetworkApplicationProfileState {
	st := NetworkApplicationProfileState{NetworkApplicationProfileArgs: args, AccountID: w.AccountID, Status: w.Status.Status,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt}
	if w.Status.Message != "" {
		m := w.Status.Message
		st.StatusMessage = &m
	}
	return st
}

func (r *NetworkApplicationProfile) Create(ctx context.Context, req infer.CreateRequest[NetworkApplicationProfileArgs]) (infer.CreateResponse[NetworkApplicationProfileState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[NetworkApplicationProfileState]{Output: NetworkApplicationProfileState{NetworkApplicationProfileArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[NetworkApplicationProfileState]{}, err
	}
	w, err := c.CreateNRAppProfile(ctx, nrAppBody(a))
	if err != nil {
		return infer.CreateResponse[NetworkApplicationProfileState]{}, fmt.Errorf("create network application profile %q: %w", a.Name, err)
	}
	if w.ID == "" {
		return infer.CreateResponse[NetworkApplicationProfileState]{}, fmt.Errorf("create network application profile %q: the response carries no id", a.Name)
	}
	return infer.CreateResponse[NetworkApplicationProfileState]{ID: w.ID, Output: nrAppStateFrom(w, a)}, nil
}

func (r *NetworkApplicationProfile) Read(ctx context.Context, req infer.ReadRequest[NetworkApplicationProfileArgs, NetworkApplicationProfileState]) (infer.ReadResponse[NetworkApplicationProfileArgs, NetworkApplicationProfileState], error) {
	type resp = infer.ReadResponse[NetworkApplicationProfileArgs, NetworkApplicationProfileState]
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	w, err := c.GetNRAppProfile(ctx, req.ID)
	if err != nil {
		return resp{}, fmt.Errorf("read network application profile %s: %w", req.ID, err)
	}
	if w == nil {
		if err := notFoundOnImport("network application profile", req.ID, req.Inputs, req.State); err != nil {
			return resp{}, err
		}
		return resp{}, nil
	}
	importing := reflect.ValueOf(req.Inputs).IsZero() && reflect.ValueOf(req.State).IsZero()
	args := nrAppArgsFrom(w, req.Inputs, importing)
	return resp{ID: req.ID, Inputs: args, State: nrAppStateFrom(w, args)}, nil
}

// Diff replaces on every input change: the API has no update call.
func (r *NetworkApplicationProfile) Diff(ctx context.Context, req infer.DiffRequest[NetworkApplicationProfileArgs, NetworkApplicationProfileState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.NetworkApplicationProfileArgs
	rep := func(name string, changed bool) {
		if changed {
			d[name] = p.PropertyDiff{Kind: p.UpdateReplace}
		}
	}
	optS := func(a, b *string) bool { return a != nil && (b == nil || *a != *b) }
	rep("businessProfileId", in.BusinessProfileID != old.BusinessProfileID)
	rep("name", in.Name != old.Name)
	rep("description", optS(in.Description, old.Description))
	rep("categoryId", in.CategoryID != old.CategoryID)
	rep("networkApis", !sameNetworkApis(in.NetworkApis, old.NetworkApis))
	rep("infrastructureLocations", !sameCountries(in.InfrastructureLocations, old.InfrastructureLocations))
	rep("privacyPolicyUrl", optS(in.PrivacyPolicyURL, old.PrivacyPolicyURL))
	rep("termsAndConditionsAccepted", in.TermsAndConditionsAccepted != nil &&
		(old.TermsAndConditionsAccepted == nil || *in.TermsAndConditionsAccepted != *old.TermsAndConditionsAccepted))
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *NetworkApplicationProfile) Delete(ctx context.Context, req infer.DeleteRequest[NetworkApplicationProfileState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.DeleteNRAppProfile(ctx, req.ID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete network application profile %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// NetworkApplicationProfileShare
// ---------------------------------------------------------------------------

// NetworkApplicationProfileShare shares an application profile with another Vonage account.
type NetworkApplicationProfileShare struct{}

// NetworkApplicationProfileShareArgs are the inputs of a share. The API can only add and remove shares,
// so changing either input replaces it.
type NetworkApplicationProfileShareArgs struct {
	ProfileID string `pulumi:"profileId"`
	AccountID string `pulumi:"accountId"`
}

func (a *NetworkApplicationProfileShareArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.ProfileID, "ID of the application profile to share, which your account must own. Changing it replaces the share.")
	an.Describe(&a.AccountID, "ID (API key) of the Vonage account to share the profile with. Changing it replaces the share.")
}

// NetworkApplicationProfileShareState is the persisted state of a share: its inputs.
type NetworkApplicationProfileShareState struct {
	NetworkApplicationProfileShareArgs
}

func (r *NetworkApplicationProfileShare) Annotate(a infer.Annotator) {
	a.Describe(&r, "Shares a Network Registry application profile with another Vonage account. The resource ID is PROFILE_ID/ACCOUNT_ID. Vonage accepts the share asynchronously (HTTP 202), so a refresh right after creation may briefly not list it. Shapes follow the published Network Registry OpenAPI definition and were not verified against a live account.")
}

var _ infer.CustomResource[NetworkApplicationProfileShareArgs, NetworkApplicationProfileShareState] = (*NetworkApplicationProfileShare)(nil)
var _ infer.CustomCheck[NetworkApplicationProfileShareArgs] = (*NetworkApplicationProfileShare)(nil)
var _ infer.CustomRead[NetworkApplicationProfileShareArgs, NetworkApplicationProfileShareState] = (*NetworkApplicationProfileShare)(nil)
var _ infer.CustomDiff[NetworkApplicationProfileShareArgs, NetworkApplicationProfileShareState] = (*NetworkApplicationProfileShare)(nil)
var _ infer.CustomDelete[NetworkApplicationProfileShareState] = (*NetworkApplicationProfileShare)(nil)

func (r *NetworkApplicationProfileShare) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[NetworkApplicationProfileShareArgs], error) {
	args, failures, err := infer.DefaultCheck[NetworkApplicationProfileShareArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[NetworkApplicationProfileShareArgs]{Inputs: args, Failures: failures}, err
	}
	for prop, v := range map[string]string{"profileId": args.ProfileID, "accountId": args.AccountID} {
		if strings.TrimSpace(v) == "" || strings.Contains(v, "/") {
			failures = append(failures, p.CheckFailure{Property: prop, Reason: prop + " must not be empty or contain a slash"})
		}
	}
	return infer.CheckResponse[NetworkApplicationProfileShareArgs]{Inputs: args, Failures: dropUnknownFailures(failures, req.NewInputs)}, nil
}

func nrShareID(profileID, accountID string) string { return profileID + "/" + accountID }

func (r *NetworkApplicationProfileShare) Create(ctx context.Context, req infer.CreateRequest[NetworkApplicationProfileShareArgs]) (infer.CreateResponse[NetworkApplicationProfileShareState], error) {
	a := req.Inputs
	st := NetworkApplicationProfileShareState{NetworkApplicationProfileShareArgs: a}
	if req.DryRun {
		return infer.CreateResponse[NetworkApplicationProfileShareState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[NetworkApplicationProfileShareState]{}, err
	}
	if err := c.ShareNRAppProfile(ctx, a.ProfileID, a.AccountID); err != nil {
		return infer.CreateResponse[NetworkApplicationProfileShareState]{}, fmt.Errorf("share network application profile %s with account %s: %w", a.ProfileID, a.AccountID, err)
	}
	return infer.CreateResponse[NetworkApplicationProfileShareState]{ID: nrShareID(a.ProfileID, a.AccountID), Output: st}, nil
}

func (r *NetworkApplicationProfileShare) Read(ctx context.Context, req infer.ReadRequest[NetworkApplicationProfileShareArgs, NetworkApplicationProfileShareState]) (infer.ReadResponse[NetworkApplicationProfileShareArgs, NetworkApplicationProfileShareState], error) {
	type resp = infer.ReadResponse[NetworkApplicationProfileShareArgs, NetworkApplicationProfileShareState]
	profileID, accountID, ok := strings.Cut(req.ID, "/")
	if !ok || profileID == "" || accountID == "" {
		return resp{}, fmt.Errorf("invalid import ID %q: expected PROFILE_ID/ACCOUNT_ID", req.ID)
	}
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	accounts, err := c.ListNRAppProfileShares(ctx, profileID)
	if err != nil && !isNotFound(err) {
		return resp{}, fmt.Errorf("read network application profile share %s: %w", req.ID, err)
	}
	if err != nil || !slices.Contains(accounts, accountID) {
		if err := notFoundOnImport("network application profile share", req.ID, req.Inputs, req.State); err != nil {
			return resp{}, err
		}
		return resp{}, nil
	}
	args := NetworkApplicationProfileShareArgs{ProfileID: profileID, AccountID: accountID}
	return resp{ID: req.ID, Inputs: args, State: NetworkApplicationProfileShareState{NetworkApplicationProfileShareArgs: args}}, nil
}

func (r *NetworkApplicationProfileShare) Diff(ctx context.Context, req infer.DiffRequest[NetworkApplicationProfileShareArgs, NetworkApplicationProfileShareState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	if req.Inputs.ProfileID != req.State.ProfileID {
		d["profileId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if req.Inputs.AccountID != req.State.AccountID {
		d["accountId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *NetworkApplicationProfileShare) Delete(ctx context.Context, req infer.DeleteRequest[NetworkApplicationProfileShareState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.UnshareNRAppProfile(ctx, req.State.ProfileID, req.State.AccountID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("unshare network application profile %s from account %s: %w", req.State.ProfileID, req.State.AccountID, err)
	}
	return infer.DeleteResponse{}, nil
}
