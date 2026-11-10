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
	"slices"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// getNetworkApplicationProfile, getNetworkApplicationProfiles, getNetworkApplicationProfileShares
// ---------------------------------------------------------------------------

// GetNetworkApplicationProfile looks up one application profile by ID.
type GetNetworkApplicationProfile struct{}

// GetNetworkApplicationProfileArgs are the lookup arguments.
type GetNetworkApplicationProfileArgs struct {
	ProfileID string `pulumi:"profileId"`
}

func (a *GetNetworkApplicationProfileArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.ProfileID, "ID of the application profile.")
}

// GetNetworkApplicationProfileResult describes an application profile.
type GetNetworkApplicationProfileResult struct {
	ProfileID               string                         `pulumi:"profileId"`
	BusinessProfileID       string                         `pulumi:"businessProfileId"`
	AccountID               string                         `pulumi:"accountId"`
	Name                    string                         `pulumi:"name"`
	Description             string                         `pulumi:"description"`
	CategoryID              string                         `pulumi:"categoryId"`
	Status                  string                         `pulumi:"status"`
	StatusMessage           string                         `pulumi:"statusMessage"`
	NetworkApis             []NetworkApplicationProfileApi `pulumi:"networkApis"`
	InfrastructureLocations []string                       `pulumi:"infrastructureLocations"`
	PrivacyPolicyURL        string                         `pulumi:"privacyPolicyUrl"`
	CreatedAt               string                         `pulumi:"createdAt"`
	UpdatedAt               string                         `pulumi:"updatedAt"`
}

func (a *GetNetworkApplicationProfileResult) Annotate(an infer.Annotator) {
	an.Describe(&a.ProfileID, "ID of the application profile.")
	an.Describe(&a.BusinessProfileID, "ID of the business profile that owns it.")
	an.Describe(&a.AccountID, "ID of the Vonage account that owns it.")
	an.Describe(&a.Name, "Name of the application.")
	an.Describe(&a.Description, "Description of the application's use cases; empty when unset.")
	an.Describe(&a.CategoryID, "Category of the application.")
	an.Describe(&a.Status, "Overall approval status: pending, provisioning, accepted, partially-accepted, rejected or failed.")
	an.Describe(&a.StatusMessage, "Vonage's note about why the profile was rejected; empty when there is none.")
	an.Describe(&a.NetworkApis, "The Network APIs the application uses and the purposes it states for each.")
	an.Describe(&a.InfrastructureLocations, "Two-letter ISO 3166 codes of the countries the application's infrastructure runs in.")
	an.Describe(&a.PrivacyPolicyURL, "URL of the application's privacy policy; empty when unset.")
	an.Describe(&a.CreatedAt, "When the profile was created (RFC 3339).")
	an.Describe(&a.UpdatedAt, "When the profile was last updated (RFC 3339).")
}

func (f *GetNetworkApplicationProfile) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Network Registry application profile by ID. A deleted profile is reported as not found.")
}

func nrAppProfileResult(w wireNRAppProfile) GetNetworkApplicationProfileResult {
	apis := []NetworkApplicationProfileApi{}
	for _, na := range w.NetworkAPIs {
		ids := na.PurposeIDs
		if ids == nil {
			ids = []string{}
		}
		apis = append(apis, NetworkApplicationProfileApi{NetworkApiID: na.NetworkAPIID, PurposeIDs: ids})
	}
	locs := w.InfrastructureLocations
	if locs == nil {
		locs = []string{}
	}
	return GetNetworkApplicationProfileResult{
		ProfileID: w.ID, BusinessProfileID: w.BusinessProfileID, AccountID: w.AccountID, Name: w.Name, Description: w.Description,
		CategoryID: w.CategoryID, Status: w.Status.Status, StatusMessage: w.Status.Message, NetworkApis: apis,
		InfrastructureLocations: locs, PrivacyPolicyURL: w.PrivacyPolicyURL, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	}
}

func (f *GetNetworkApplicationProfile) Invoke(ctx context.Context, req infer.FunctionRequest[GetNetworkApplicationProfileArgs]) (infer.FunctionResponse[GetNetworkApplicationProfileResult], error) {
	id := req.Input.ProfileID
	if id == "" {
		return infer.FunctionResponse[GetNetworkApplicationProfileResult]{}, fmt.Errorf("getNetworkApplicationProfile: profileId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNetworkApplicationProfileResult]{}, err
	}
	w, err := c.GetNRAppProfile(ctx, id)
	if err != nil {
		return infer.FunctionResponse[GetNetworkApplicationProfileResult]{}, fmt.Errorf("getNetworkApplicationProfile: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[GetNetworkApplicationProfileResult]{}, fmt.Errorf("getNetworkApplicationProfile: application profile %q was not found", id)
	}
	return infer.FunctionResponse[GetNetworkApplicationProfileResult]{Output: nrAppProfileResult(*w)}, nil
}

// GetNetworkApplicationProfiles lists application profiles.
type GetNetworkApplicationProfiles struct{}

// GetNetworkApplicationProfilesArgs are the lookup arguments.
type GetNetworkApplicationProfilesArgs struct {
	BusinessProfileID *string `pulumi:"businessProfileId,optional"`
	Status            *string `pulumi:"status,optional"`
	Shared            *bool   `pulumi:"shared,optional"`
}

func (a *GetNetworkApplicationProfilesArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BusinessProfileID, "Only list the profiles of this business profile.")
	an.Describe(&a.Status, "Only list profiles with this overall status (pending, provisioning, accepted, partially-accepted, rejected or failed).")
	an.Describe(&a.Shared, "List the profiles other accounts have shared with this one instead of the account's own. Defaults to false.")
}

// GetNetworkApplicationProfilesResult lists the profiles.
type GetNetworkApplicationProfilesResult struct {
	Profiles []GetNetworkApplicationProfileResult `pulumi:"profiles"`
}

func (a *GetNetworkApplicationProfilesResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Profiles, "The application profiles, deleted ones excluded.")
}

func (f *GetNetworkApplicationProfiles) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the account's Network Registry application profiles, following every page.")
}

func (f *GetNetworkApplicationProfiles) Invoke(ctx context.Context, req infer.FunctionRequest[GetNetworkApplicationProfilesArgs]) (infer.FunctionResponse[GetNetworkApplicationProfilesResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNetworkApplicationProfilesResult]{}, err
	}
	ws, err := c.ListNRAppProfiles(ctx, deref(req.Input.BusinessProfileID), deref(req.Input.Status), effective(req.Input.Shared, false))
	if err != nil {
		return infer.FunctionResponse[GetNetworkApplicationProfilesResult]{}, fmt.Errorf("getNetworkApplicationProfiles: %w", err)
	}
	out := GetNetworkApplicationProfilesResult{Profiles: []GetNetworkApplicationProfileResult{}}
	for _, w := range ws {
		out.Profiles = append(out.Profiles, nrAppProfileResult(w))
	}
	return infer.FunctionResponse[GetNetworkApplicationProfilesResult]{Output: out}, nil
}

// GetNetworkApplicationProfileShares lists the accounts an application profile is shared with.
type GetNetworkApplicationProfileShares struct{}

// GetNetworkApplicationProfileSharesArgs are the lookup arguments.
type GetNetworkApplicationProfileSharesArgs struct {
	ProfileID string `pulumi:"profileId"`
}

func (a *GetNetworkApplicationProfileSharesArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.ProfileID, "ID of the application profile, which your account must own.")
}

// GetNetworkApplicationProfileSharesResult lists the accounts.
type GetNetworkApplicationProfileSharesResult struct {
	AccountIDs []string `pulumi:"accountIds"`
}

func (a *GetNetworkApplicationProfileSharesResult) Annotate(an infer.Annotator) {
	an.Describe(&a.AccountIDs, "IDs of the Vonage accounts the profile is shared with.")
}

func (f *GetNetworkApplicationProfileShares) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the Vonage accounts a Network Registry application profile has been shared with, following every page.")
}

func (f *GetNetworkApplicationProfileShares) Invoke(ctx context.Context, req infer.FunctionRequest[GetNetworkApplicationProfileSharesArgs]) (infer.FunctionResponse[GetNetworkApplicationProfileSharesResult], error) {
	id := req.Input.ProfileID
	if id == "" {
		return infer.FunctionResponse[GetNetworkApplicationProfileSharesResult]{}, fmt.Errorf("getNetworkApplicationProfileShares: profileId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNetworkApplicationProfileSharesResult]{}, err
	}
	ids, err := c.ListNRAppProfileShares(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return infer.FunctionResponse[GetNetworkApplicationProfileSharesResult]{}, fmt.Errorf("getNetworkApplicationProfileShares: application profile %q was not found", id)
		}
		return infer.FunctionResponse[GetNetworkApplicationProfileSharesResult]{}, fmt.Errorf("getNetworkApplicationProfileShares: %w", err)
	}
	return infer.FunctionResponse[GetNetworkApplicationProfileSharesResult]{Output: GetNetworkApplicationProfileSharesResult{AccountIDs: ids}}, nil
}

// GetNetworkApplicationProfileShare looks up one share of an application profile.
type GetNetworkApplicationProfileShare struct{}

// GetNetworkApplicationProfileShareArgs are the lookup arguments.
type GetNetworkApplicationProfileShareArgs struct {
	ProfileID string `pulumi:"profileId"`
	AccountID string `pulumi:"accountId"`
}

func (a *GetNetworkApplicationProfileShareArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.ProfileID, "ID of the application profile, which your account must own.")
	an.Describe(&a.AccountID, "ID (API key) of the Vonage account the profile is shared with.")
}

// GetNetworkApplicationProfileShareResult confirms the share.
type GetNetworkApplicationProfileShareResult struct {
	ProfileID string `pulumi:"profileId"`
	AccountID string `pulumi:"accountId"`
}

func (a *GetNetworkApplicationProfileShareResult) Annotate(an infer.Annotator) {
	an.Describe(&a.ProfileID, "ID of the application profile.")
	an.Describe(&a.AccountID, "ID of the Vonage account the profile is shared with.")
}

func (f *GetNetworkApplicationProfileShare) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up whether a Network Registry application profile is shared with an account. The API has no get-one call, so this lists the profile's shares and picks the match; an absent share is reported as not found.")
}

func (f *GetNetworkApplicationProfileShare) Invoke(ctx context.Context, req infer.FunctionRequest[GetNetworkApplicationProfileShareArgs]) (infer.FunctionResponse[GetNetworkApplicationProfileShareResult], error) {
	id, account := req.Input.ProfileID, req.Input.AccountID
	if id == "" || account == "" {
		return infer.FunctionResponse[GetNetworkApplicationProfileShareResult]{}, fmt.Errorf("getNetworkApplicationProfileShare: profileId and accountId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNetworkApplicationProfileShareResult]{}, err
	}
	accounts, err := c.ListNRAppProfileShares(ctx, id)
	if err != nil && !isNotFound(err) {
		return infer.FunctionResponse[GetNetworkApplicationProfileShareResult]{}, fmt.Errorf("getNetworkApplicationProfileShare: %w", err)
	}
	if err != nil || !slices.Contains(accounts, account) {
		return infer.FunctionResponse[GetNetworkApplicationProfileShareResult]{}, fmt.Errorf("getNetworkApplicationProfileShare: application profile %q is not shared with account %q (or was not found)", id, account)
	}
	return infer.FunctionResponse[GetNetworkApplicationProfileShareResult]{Output: GetNetworkApplicationProfileShareResult{ProfileID: id, AccountID: account}}, nil
}
