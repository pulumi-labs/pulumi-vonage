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
	"strings"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// getNetworkBusinessProfile, getNetworkBusinessProfiles
// ---------------------------------------------------------------------------

// GetNetworkBusinessProfile looks up one business profile by ID.
type GetNetworkBusinessProfile struct{}

// GetNetworkBusinessProfileArgs are the lookup arguments.
type GetNetworkBusinessProfileArgs struct {
	ProfileID string `pulumi:"profileId"`
}

func (a *GetNetworkBusinessProfileArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.ProfileID, "ID of the business profile.")
}

// GetNetworkBusinessProfileResult describes a business profile.
type GetNetworkBusinessProfileResult struct {
	ProfileID                 string                     `pulumi:"profileId"`
	AccountID                 string                     `pulumi:"accountId"`
	Name                      string                     `pulumi:"name"`
	TradingName               string                     `pulumi:"tradingName"`
	Status                    string                     `pulumi:"status"`
	StatusMessage             string                     `pulumi:"statusMessage"`
	Countries                 []string                   `pulumi:"countries"`
	OrganisationType          string                     `pulumi:"organisationType"`
	ContactRole               string                     `pulumi:"contactRole"`
	TaxNumber                 string                     `pulumi:"taxNumber"`
	PrivacyPolicyURL          string                     `pulumi:"privacyPolicyUrl"`
	PrivacyPolicyManagerEmail string                     `pulumi:"privacyPolicyManagerEmail"`
	BusinessInformation       NetworkBusinessInformation `pulumi:"businessInformation"`
	CreatedAt                 string                     `pulumi:"createdAt"`
	UpdatedAt                 string                     `pulumi:"updatedAt"`
}

func (a *GetNetworkBusinessProfileResult) Annotate(an infer.Annotator) {
	an.Describe(&a.ProfileID, "Profile ID.")
	an.Describe(&a.AccountID, "ID of the account that owns the profile.")
	an.Describe(&a.Name, "Legal name of the business.")
	an.Describe(&a.TradingName, "Trading name, empty when unset.")
	an.Describe(&a.Status, "Overall registration status.")
	an.Describe(&a.StatusMessage, "Vonage's note about the status, empty when there is none.")
	an.Describe(&a.Countries, "Countries the profile is registered for.")
	an.Describe(&a.OrganisationType, "Kind of organisation.")
	an.Describe(&a.ContactRole, "Role of the contact person.")
	an.Describe(&a.TaxNumber, "Tax identification number.")
	an.Describe(&a.PrivacyPolicyURL, "Privacy policy URL.")
	an.Describe(&a.PrivacyPolicyManagerEmail, "Privacy policy manager email.")
	an.Describe(&a.BusinessInformation, "Postal address.")
	an.Describe(&a.CreatedAt, "Creation time (RFC 3339).")
	an.Describe(&a.UpdatedAt, "Last update time (RFC 3339).")
}

func (f *GetNetworkBusinessProfile) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Network Registry business profile by ID. Fails if it does not exist or has been deleted.")
}

func nrProfileResult(w wireNRBusinessProfile) GetNetworkBusinessProfileResult {
	e := w.effective()
	s := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	countries := e.Countries
	if countries == nil {
		countries = []string{}
	}
	return GetNetworkBusinessProfileResult{ProfileID: w.ID, AccountID: w.AccountID, Name: e.Name, TradingName: e.TradingName,
		Status: w.Status.Status, StatusMessage: w.Status.Message, Countries: countries, OrganisationType: e.OrganisationType,
		ContactRole: e.ContactRole, TaxNumber: e.TaxNumber, PrivacyPolicyURL: e.PrivacyPolicyURL,
		PrivacyPolicyManagerEmail: e.PrivacyPolicyManagerEmail,
		BusinessInformation: NetworkBusinessInformation{AddressStreet1: e.BusinessInformation.AddressStreet1,
			AddressStreet2: s(e.BusinessInformation.AddressStreet2), PostalCode: e.BusinessInformation.PostalCode,
			Country: e.BusinessInformation.Country, City: e.BusinessInformation.City, Province: s(e.BusinessInformation.Province)},
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt}
}

func (f *GetNetworkBusinessProfile) Invoke(ctx context.Context, req infer.FunctionRequest[GetNetworkBusinessProfileArgs]) (infer.FunctionResponse[GetNetworkBusinessProfileResult], error) {
	id := req.Input.ProfileID
	if id == "" {
		return infer.FunctionResponse[GetNetworkBusinessProfileResult]{}, fmt.Errorf("getNetworkBusinessProfile: profileId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNetworkBusinessProfileResult]{}, err
	}
	w, err := c.GetNRBusinessProfile(ctx, id)
	if err != nil {
		return infer.FunctionResponse[GetNetworkBusinessProfileResult]{}, fmt.Errorf("getNetworkBusinessProfile: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[GetNetworkBusinessProfileResult]{}, fmt.Errorf("getNetworkBusinessProfile: business profile %q was not found", id)
	}
	if w.ID == "" {
		w.ID = id
	}
	return infer.FunctionResponse[GetNetworkBusinessProfileResult]{Output: nrProfileResult(*w)}, nil
}

// GetNetworkBusinessProfiles lists the account's business profiles.
type GetNetworkBusinessProfiles struct{}

// GetNetworkBusinessProfilesArgs are the lookup arguments.
type GetNetworkBusinessProfilesArgs struct {
	Shared *bool `pulumi:"shared,optional"`
}

func (a *GetNetworkBusinessProfilesArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Shared, "List the profiles other accounts have shared with this one instead of the account's own. Defaults to false.")
}

// GetNetworkBusinessProfilesResult lists the profiles.
type GetNetworkBusinessProfilesResult struct {
	Profiles []GetNetworkBusinessProfileResult `pulumi:"profiles"`
}

func (a *GetNetworkBusinessProfilesResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Profiles, "The business profiles, deleted ones excluded.")
}

func (f *GetNetworkBusinessProfiles) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the account's Network Registry business profiles, following every page.")
}

func (f *GetNetworkBusinessProfiles) Invoke(ctx context.Context, req infer.FunctionRequest[GetNetworkBusinessProfilesArgs]) (infer.FunctionResponse[GetNetworkBusinessProfilesResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNetworkBusinessProfilesResult]{}, err
	}
	ws, err := c.ListNRBusinessProfiles(ctx, effective(req.Input.Shared, false))
	if err != nil {
		return infer.FunctionResponse[GetNetworkBusinessProfilesResult]{}, fmt.Errorf("getNetworkBusinessProfiles: %w", err)
	}
	out := GetNetworkBusinessProfilesResult{Profiles: []GetNetworkBusinessProfileResult{}}
	for _, w := range ws {
		out.Profiles = append(out.Profiles, nrProfileResult(w))
	}
	return infer.FunctionResponse[GetNetworkBusinessProfilesResult]{Output: out}, nil
}

// ---------------------------------------------------------------------------
// Catalogue: getNetworkRegistryCatalogue, getNetworkTermsAndConditions
// ---------------------------------------------------------------------------

// GetNetworkRegistryCatalogue reads the Network Registry reference data.
type GetNetworkRegistryCatalogue struct{}

// GetNetworkRegistryCatalogueArgs are the lookup arguments.
type GetNetworkRegistryCatalogueArgs struct {
	Countries []string `pulumi:"countries,optional"`
	All       *bool    `pulumi:"all,optional"`
}

func (a *GetNetworkRegistryCatalogueArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Countries, "Only list the CSPs and Network APIs available in these two-letter ISO 3166 country codes.")
	an.Describe(&a.All, "List every CSP and Network API defined on the platform, not only the ones enabled for the account. Defaults to false.")
}

// NetworkRegistryCountry is a country of the catalogue.
type NetworkRegistryCountry struct {
	Code string `pulumi:"code"`
	Name string `pulumi:"name"`
}

func (a *NetworkRegistryCountry) Annotate(an infer.Annotator) {
	an.Describe(&a.Code, "Two-letter ISO 3166 country code.")
	an.Describe(&a.Name, "Country name.")
}

// NetworkRegistryEntry is a catalogue entry with an ID and a name.
type NetworkRegistryEntry struct {
	ID   string `pulumi:"id"`
	Name string `pulumi:"name"`
}

func (a *NetworkRegistryEntry) Annotate(an infer.Annotator) {
	an.Describe(&a.ID, "Identifier to use in the API, for example an organisation type or a category.")
	an.Describe(&a.Name, "Human-readable name.")
}

// NetworkRegistryCsp is a communication service provider (a mobile operator).
type NetworkRegistryCsp struct {
	ID                   string   `pulumi:"id"`
	Name                 string   `pulumi:"name"`
	Country              string   `pulumi:"country"`
	SupportedNetworkApis []string `pulumi:"supportedNetworkApis"`
	VettingLevel         string   `pulumi:"vettingLevel"`
}

func (a *NetworkRegistryCsp) Annotate(an infer.Annotator) {
	an.Describe(&a.ID, "CSP identifier, for example telefonica_es.")
	an.Describe(&a.Name, "Operator name.")
	an.Describe(&a.Country, "Two-letter ISO 3166 country code the CSP operates in.")
	an.Describe(&a.SupportedNetworkApis, "IDs of the Network APIs the CSP supports.")
	an.Describe(&a.VettingLevel, "ID of the vetting level the CSP requires, empty when none.")
}

// NetworkRegistryNetworkApi is a Network API.
type NetworkRegistryNetworkApi struct {
	ID       string   `pulumi:"id"`
	Name     string   `pulumi:"name"`
	Requires []string `pulumi:"requires"`
}

func (a *NetworkRegistryNetworkApi) Annotate(an infer.Annotator) {
	an.Describe(&a.ID, "Network API identifier, for example sim-swap.")
	an.Describe(&a.Name, "Network API name.")
	an.Describe(&a.Requires, "Callback URLs an application profile must provide to use the API: nv_redirect_url, qod_callback_url or verify_callback_url.")
}

// NetworkRegistryPurpose is a purpose a Network API can be used for.
type NetworkRegistryPurpose struct {
	ID           string `pulumi:"id"`
	Name         string `pulumi:"name"`
	NetworkApiID string `pulumi:"networkApiId"`
}

func (a *NetworkRegistryPurpose) Annotate(an infer.Annotator) {
	an.Describe(&a.ID, "Purpose identifier.")
	an.Describe(&a.Name, "Purpose name.")
	an.Describe(&a.NetworkApiID, "ID of the Network API the purpose belongs to.")
}

// NetworkRegistryVettingLevel is a vetting level.
type NetworkRegistryVettingLevel struct {
	ID          string   `pulumi:"id"`
	Name        string   `pulumi:"name"`
	Description string   `pulumi:"description"`
	ExtraFields []string `pulumi:"extraFields"`
}

func (a *NetworkRegistryVettingLevel) Annotate(an infer.Annotator) {
	an.Describe(&a.ID, "Vetting level identifier.")
	an.Describe(&a.Name, "Vetting level name.")
	an.Describe(&a.Description, "What the level checks.")
	an.Describe(&a.ExtraFields, "Names of the vettingExtraFields a business profile must provide when the level applies.")
}

// GetNetworkRegistryCatalogueResult holds every reference list.
type GetNetworkRegistryCatalogueResult struct {
	Countries          []NetworkRegistryCountry      `pulumi:"countries"`
	OrganisationTypes  []NetworkRegistryEntry        `pulumi:"organisationTypes"`
	Categories         []NetworkRegistryEntry        `pulumi:"categories"`
	Csps               []NetworkRegistryCsp          `pulumi:"csps"`
	NetworkApis        []NetworkRegistryNetworkApi   `pulumi:"networkApis"`
	Purposes           []NetworkRegistryPurpose      `pulumi:"purposes"`
	VettingLevels      []NetworkRegistryVettingLevel `pulumi:"vettingLevels"`
	TermsAndConditions []NetworkRegistryEntry        `pulumi:"termsAndConditions"`
}

func (a *GetNetworkRegistryCatalogueResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Countries, "Countries a business profile can be registered for.")
	an.Describe(&a.OrganisationTypes, "Valid organisationType values of a business profile.")
	an.Describe(&a.Categories, "Valid application profile categories.")
	an.Describe(&a.Csps, "Communication service providers (mobile operators).")
	an.Describe(&a.NetworkApis, "Network APIs and the callback URLs each requires.")
	an.Describe(&a.Purposes, "Purposes Network APIs can be used for.")
	an.Describe(&a.VettingLevels, "Vetting levels and the extra fields each needs.")
	an.Describe(&a.TermsAndConditions, "Terms and conditions documents; read the text with getNetworkTermsAndConditions.")
}

func (f *GetNetworkRegistryCatalogue) Annotate(a infer.Annotator) {
	a.Describe(&f, "Read the Network Registry reference data (countries, organisation types, categories, CSPs, Network APIs, purposes, vetting levels and terms and conditions), following every page. Read-only and free.")
}

func (f *GetNetworkRegistryCatalogue) Invoke(ctx context.Context, req infer.FunctionRequest[GetNetworkRegistryCatalogueArgs]) (infer.FunctionResponse[GetNetworkRegistryCatalogueResult], error) {
	type resp = infer.FunctionResponse[GetNetworkRegistryCatalogueResult]
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	filter := url.Values{}
	if len(req.Input.Countries) > 0 {
		filter.Set("countries", strings.ToUpper(strings.Join(req.Input.Countries, ",")))
	}
	if effective(req.Input.All, false) {
		filter.Set("all", "true")
	}
	var out GetNetworkRegistryCatalogueResult
	cat := nrBase + "/catalogue/"
	wrap := func(what string, err error) error {
		return fmt.Errorf("getNetworkRegistryCatalogue: %s: %w", what, err)
	}

	countries, err := nrList[struct{ Code, Name string }](ctx, c, cat+"countries", nil, "countries")
	if err != nil {
		return resp{}, wrap("countries", err)
	}
	out.Countries = []NetworkRegistryCountry{}
	for _, x := range countries {
		out.Countries = append(out.Countries, NetworkRegistryCountry{Code: x.Code, Name: x.Name})
	}
	entries := func(path, key string) ([]NetworkRegistryEntry, error) {
		xs, err := nrList[struct{ ID, Name string }](ctx, c, cat+path, nil, key)
		if err != nil {
			return nil, wrap(path, err)
		}
		res := []NetworkRegistryEntry{}
		for _, x := range xs {
			res = append(res, NetworkRegistryEntry{ID: x.ID, Name: x.Name})
		}
		return res, nil
	}
	if out.OrganisationTypes, err = entries("organisation-types", "organisation_types"); err != nil {
		return resp{}, err
	}
	if out.Categories, err = entries("categories", "categories"); err != nil {
		return resp{}, err
	}
	if out.TermsAndConditions, err = entries("terms-and-conditions", "terms_and_conditions"); err != nil {
		return resp{}, err
	}
	csps, err := nrList[struct {
		ID                   string   `json:"id"`
		Name                 string   `json:"name"`
		Country              string   `json:"country"`
		SupportedNetworkAPIs []string `json:"supported_network_apis"`
		VettingLevel         string   `json:"vetting_level"`
	}](ctx, c, cat+"csps", filter, "csps")
	if err != nil {
		return resp{}, wrap("csps", err)
	}
	out.Csps = []NetworkRegistryCsp{}
	for _, x := range csps {
		apis := x.SupportedNetworkAPIs
		if apis == nil {
			apis = []string{}
		}
		out.Csps = append(out.Csps, NetworkRegistryCsp{ID: x.ID, Name: x.Name, Country: x.Country, SupportedNetworkApis: apis, VettingLevel: x.VettingLevel})
	}
	apis, err := nrList[struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Requires []string `json:"requires"`
	}](ctx, c, cat+"network-apis", filter, "network_apis")
	if err != nil {
		return resp{}, wrap("network-apis", err)
	}
	out.NetworkApis = []NetworkRegistryNetworkApi{}
	for _, x := range apis {
		rq := x.Requires
		if rq == nil {
			rq = []string{}
		}
		out.NetworkApis = append(out.NetworkApis, NetworkRegistryNetworkApi{ID: x.ID, Name: x.Name, Requires: rq})
	}
	purposes, err := nrList[struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		NetworkAPIID string `json:"network_api_id"`
	}](ctx, c, cat+"purposes", nil, "purposes")
	if err != nil {
		return resp{}, wrap("purposes", err)
	}
	out.Purposes = []NetworkRegistryPurpose{}
	for _, x := range purposes {
		out.Purposes = append(out.Purposes, NetworkRegistryPurpose{ID: x.ID, Name: x.Name, NetworkApiID: x.NetworkAPIID})
	}
	levels, err := nrList[struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		ExtraFields []string `json:"extra_fields"`
	}](ctx, c, cat+"vetting-levels", nil, "vetting_levels")
	if err != nil {
		return resp{}, wrap("vetting-levels", err)
	}
	out.VettingLevels = []NetworkRegistryVettingLevel{}
	for _, x := range levels {
		ef := x.ExtraFields
		if ef == nil {
			ef = []string{}
		}
		out.VettingLevels = append(out.VettingLevels, NetworkRegistryVettingLevel{ID: x.ID, Name: x.Name, Description: x.Description, ExtraFields: ef})
	}
	return resp{Output: out}, nil
}

// GetNetworkTermsAndConditions reads one terms and conditions document.
type GetNetworkTermsAndConditions struct{}

// GetNetworkTermsAndConditionsArgs are the lookup arguments.
type GetNetworkTermsAndConditionsArgs struct {
	DocumentID string `pulumi:"documentId"`
}

func (a *GetNetworkTermsAndConditionsArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.DocumentID, "ID of the document, as listed in getNetworkRegistryCatalogue termsAndConditions.")
}

// GetNetworkTermsAndConditionsResult is the document.
type GetNetworkTermsAndConditionsResult struct {
	DocumentID string `pulumi:"documentId"`
	Name       string `pulumi:"name"`
	Text       string `pulumi:"text"`
}

func (a *GetNetworkTermsAndConditionsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.DocumentID, "Document ID.")
	an.Describe(&a.Name, "Document name.")
	an.Describe(&a.Text, "Full text of the document.")
}

func (f *GetNetworkTermsAndConditions) Annotate(a infer.Annotator) {
	a.Describe(&f, "Read one Network Registry terms and conditions document by ID. Fails if it does not exist.")
}

func (f *GetNetworkTermsAndConditions) Invoke(ctx context.Context, req infer.FunctionRequest[GetNetworkTermsAndConditionsArgs]) (infer.FunctionResponse[GetNetworkTermsAndConditionsResult], error) {
	id := req.Input.DocumentID
	if id == "" {
		return infer.FunctionResponse[GetNetworkTermsAndConditionsResult]{}, fmt.Errorf("getNetworkTermsAndConditions: documentId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNetworkTermsAndConditionsResult]{}, err
	}
	var raw struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Text string `json:"text"`
	}
	err = c.apiJSON(ctx, http.MethodGet, nrBase+"/catalogue/terms-and-conditions/"+url.PathEscape(id), nil, nil, &raw)
	if isNotFound(err) {
		return infer.FunctionResponse[GetNetworkTermsAndConditionsResult]{}, fmt.Errorf("getNetworkTermsAndConditions: document %q was not found", id)
	}
	if err != nil {
		return infer.FunctionResponse[GetNetworkTermsAndConditionsResult]{}, fmt.Errorf("getNetworkTermsAndConditions: %w", err)
	}
	if raw.ID == "" {
		raw.ID = id
	}
	return infer.FunctionResponse[GetNetworkTermsAndConditionsResult]{Output: GetNetworkTermsAndConditionsResult{DocumentID: raw.ID, Name: raw.Name, Text: raw.Text}}, nil
}
