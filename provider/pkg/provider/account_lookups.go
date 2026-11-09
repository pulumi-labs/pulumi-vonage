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
	"strconv"
	"strings"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// getAccountBalance
// ---------------------------------------------------------------------------

// GetAccountBalance reads the prepaid balance of the provider's account.
type GetAccountBalance struct{}

// GetAccountBalanceArgs has no arguments.
type GetAccountBalanceArgs struct{}

// GetAccountBalanceResult is the account balance.
type GetAccountBalanceResult struct {
	Value      float64 `pulumi:"value"`
	AutoReload bool    `pulumi:"autoReload"`
}

func (a *GetAccountBalanceResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Value, "Account balance in euros.")
	an.Describe(&a.AutoReload, "Whether auto-reload is enabled on the account.")
}

func (f *GetAccountBalance) Annotate(a infer.Annotator) {
	a.Describe(&f, "Read the balance of the provider's Vonage account (GET /account/get-balance).")
}

type wireBalance struct {
	Value      float64 `json:"value"`
	AutoReload bool    `json:"autoReload"`
}

func (f *GetAccountBalance) Invoke(ctx context.Context, _ infer.FunctionRequest[GetAccountBalanceArgs]) (infer.FunctionResponse[GetAccountBalanceResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetAccountBalanceResult]{}, err
	}
	var out wireBalance
	if err := c.restForm(ctx, http.MethodGet, "/account/get-balance", nil, nil, &out); err != nil {
		return infer.FunctionResponse[GetAccountBalanceResult]{}, fmt.Errorf("getAccountBalance: %w", err)
	}
	return infer.FunctionResponse[GetAccountBalanceResult]{Output: GetAccountBalanceResult{Value: out.Value, AutoReload: out.AutoReload}}, nil
}

// ---------------------------------------------------------------------------
// getNumbers: every number the account owns
// ---------------------------------------------------------------------------

// GetNumbers lists the numbers owned by the account.
type GetNumbers struct{}

// GetNumbersArgs filters the list.
type GetNumbersArgs struct {
	Country        *string `pulumi:"country,optional"`
	Pattern        *string `pulumi:"pattern,optional"`
	SearchPattern  *int    `pulumi:"searchPattern,optional"`
	ApplicationID  *string `pulumi:"applicationId,optional"`
	HasApplication *bool   `pulumi:"hasApplication,optional"`
}

func (a *GetNumbersArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Country, "Only numbers in this two-letter ISO country code, for example US.")
	an.Describe(&a.Pattern, "Only numbers matching this digit pattern.")
	an.Describe(&a.SearchPattern, "How pattern matches: 0 starts with, 1 contains (the default), 2 ends with.")
	an.Describe(&a.ApplicationID, "Only numbers linked to this application ID.")
	an.Describe(&a.HasApplication, "Only numbers that are (true) or are not (false) linked to an application.")
}

// OwnedNumberSummary is one owned number.
type OwnedNumberSummary struct {
	Country               string   `pulumi:"country"`
	Msisdn                string   `pulumi:"msisdn"`
	Type                  string   `pulumi:"type"`
	Features              []string `pulumi:"features"`
	AppID                 string   `pulumi:"appId"`
	MoHTTPURL             string   `pulumi:"moHttpUrl"`
	VoiceCallbackType     string   `pulumi:"voiceCallbackType"`
	VoiceCallbackValue    string   `pulumi:"voiceCallbackValue"`
	MessagesCallbackType  string   `pulumi:"messagesCallbackType"`
	MessagesCallbackValue string   `pulumi:"messagesCallbackValue"`
}

func (a *OwnedNumberSummary) Annotate(an infer.Annotator) {
	an.Describe(&a.Country, "Country code.")
	an.Describe(&a.Msisdn, "The number in international format without a leading plus.")
	an.Describe(&a.Type, "Number type (landline, mobile-lvn, landline-toll-free).")
	an.Describe(&a.Features, "Capabilities: SMS, VOICE, MMS.")
	an.Describe(&a.AppID, "Linked application ID, empty if none.")
	an.Describe(&a.MoHTTPURL, "Inbound SMS webhook, empty if none.")
	an.Describe(&a.VoiceCallbackType, "Voice routing type.")
	an.Describe(&a.VoiceCallbackValue, "Voice routing target.")
	an.Describe(&a.MessagesCallbackType, "Messages routing type.")
	an.Describe(&a.MessagesCallbackValue, "Messages routing target.")
}

// GetNumbersResult lists owned numbers.
type GetNumbersResult struct {
	Numbers []OwnedNumberSummary `pulumi:"numbers"`
}

func (a *GetNumbersResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Numbers, "The matching numbers, in the order Vonage lists them.")
}

func (f *GetNumbers) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the numbers owned by the account (GET /account/numbers), following every page.")
}

// maxNumberPages bounds the pages getNumbers follows (100 numbers per page).
const maxNumberPages = 100

func (f *GetNumbers) Invoke(ctx context.Context, req infer.FunctionRequest[GetNumbersArgs]) (infer.FunctionResponse[GetNumbersResult], error) {
	in := req.Input
	if in.SearchPattern != nil && (*in.SearchPattern < 0 || *in.SearchPattern > 2) {
		return infer.FunctionResponse[GetNumbersResult]{}, fmt.Errorf("getNumbers: searchPattern must be 0, 1 or 2")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNumbersResult]{}, err
	}
	out := GetNumbersResult{Numbers: []OwnedNumberSummary{}}
	for index := 1; index <= maxNumberPages; index++ {
		q := url.Values{"size": {"100"}, "index": {strconv.Itoa(index)}}
		setQ(q, "country", in.Country)
		setQ(q, "pattern", in.Pattern)
		setQ(q, "application_id", in.ApplicationID)
		if in.SearchPattern != nil {
			q.Set("search_pattern", strconv.Itoa(*in.SearchPattern))
		}
		if in.HasApplication != nil {
			q.Set("has_application", strconv.FormatBool(*in.HasApplication))
		}
		var page wireOwnedNumbers
		if err := c.restForm(ctx, http.MethodGet, "/account/numbers", q, nil, &page); err != nil {
			return infer.FunctionResponse[GetNumbersResult]{}, fmt.Errorf("getNumbers: %w", err)
		}
		for _, n := range page.Numbers {
			out.Numbers = append(out.Numbers, OwnedNumberSummary{
				Country: n.Country, Msisdn: n.MSISDN, Type: n.Type, Features: nonNilStrings(n.Features), AppID: n.AppID,
				MoHTTPURL: n.MoHTTPURL, VoiceCallbackType: n.VoiceCallbackType, VoiceCallbackValue: n.VoiceCallbackValue,
				MessagesCallbackType: n.MessagesCallbackType, MessagesCallbackValue: n.MessagesCallbackValue,
			})
		}
		more, err := page.hasMore(index)
		if err != nil {
			return infer.FunctionResponse[GetNumbersResult]{}, fmt.Errorf("getNumbers: %w", err)
		}
		if !more {
			return infer.FunctionResponse[GetNumbersResult]{Output: out}, nil
		}
	}
	return infer.FunctionResponse[GetNumbersResult]{}, fmt.Errorf("getNumbers: more than %d numbers match; narrow the filters", maxNumberPages*100)
}

func setQ(q url.Values, key string, v *string) {
	if v != nil && *v != "" {
		q.Set(key, *v)
	}
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ---------------------------------------------------------------------------
// getAvailableNumbers: numbers that can be bought
// ---------------------------------------------------------------------------

// GetAvailableNumbers searches the numbers Vonage has available to buy. It never buys one.
type GetAvailableNumbers struct{}

// GetAvailableNumbersArgs are the search filters.
type GetAvailableNumbersArgs struct {
	Country       string   `pulumi:"country"`
	Type          *string  `pulumi:"type,optional"`
	Pattern       *string  `pulumi:"pattern,optional"`
	SearchPattern *int     `pulumi:"searchPattern,optional"`
	Features      []string `pulumi:"features,optional"`
	Size          *int     `pulumi:"size,optional"`
	Index         *int     `pulumi:"index,optional"`
}

func (a *GetAvailableNumbersArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Country, "Two-letter ISO country code, for example US.")
	an.Describe(&a.Type, "Number type: landline, mobile-lvn or landline-toll-free.")
	an.Describe(&a.Pattern, "Digit pattern the number must match.")
	an.Describe(&a.SearchPattern, "How pattern matches: 0 starts with, 1 contains (the default), 2 ends with.")
	an.Describe(&a.Features, "Capabilities the number must have: SMS, VOICE and MMS.")
	an.Describe(&a.Size, "Page size, 1 to 100 (Vonage default 10).")
	an.Describe(&a.Index, "Page number, starting at 1.")
}

// AvailableNumber is one number that can be bought.
type AvailableNumber struct {
	Country  string   `pulumi:"country"`
	Msisdn   string   `pulumi:"msisdn"`
	Type     string   `pulumi:"type"`
	Cost     string   `pulumi:"cost"`
	Features []string `pulumi:"features"`
}

func (a *AvailableNumber) Annotate(an infer.Annotator) {
	an.Describe(&a.Country, "Country code.")
	an.Describe(&a.Msisdn, "The number in international format without a leading plus.")
	an.Describe(&a.Type, "Number type.")
	an.Describe(&a.Cost, "Monthly rental in euros, as Vonage reports it (a decimal string).")
	an.Describe(&a.Features, "Capabilities: SMS, VOICE, MMS.")
}

// GetAvailableNumbersResult is one page of search results.
type GetAvailableNumbersResult struct {
	Count   int               `pulumi:"count"`
	Numbers []AvailableNumber `pulumi:"numbers"`
}

func (a *GetAvailableNumbersResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Count, "Total number of matches Vonage reports (all pages).")
	an.Describe(&a.Numbers, "The numbers on the requested page.")
}

func (f *GetAvailableNumbers) Annotate(a infer.Annotator) {
	a.Describe(&f, "Search the numbers available to buy in a country (GET /number/search). Read-only: it never buys a number, and a Number resource still has to.")
}

type wireAvailableNumbers struct {
	Count   int `json:"count"`
	Numbers []struct {
		Country  string   `json:"country"`
		MSISDN   string   `json:"msisdn"`
		Type     string   `json:"type"`
		Cost     string   `json:"cost"`
		Features []string `json:"features"`
	} `json:"numbers"`
}

func (f *GetAvailableNumbers) Invoke(ctx context.Context, req infer.FunctionRequest[GetAvailableNumbersArgs]) (infer.FunctionResponse[GetAvailableNumbersResult], error) {
	in := req.Input
	fail := func(format string, a ...any) (infer.FunctionResponse[GetAvailableNumbersResult], error) {
		return infer.FunctionResponse[GetAvailableNumbersResult]{}, fmt.Errorf("getAvailableNumbers: "+format, a...)
	}
	if len(in.Country) != 2 {
		return fail("country must be a two-letter ISO country code")
	}
	if in.SearchPattern != nil && (*in.SearchPattern < 0 || *in.SearchPattern > 2) {
		return fail("searchPattern must be 0, 1 or 2")
	}
	if in.Size != nil && (*in.Size < 1 || *in.Size > 100) {
		return fail("size must be between 1 and 100")
	}
	if in.Index != nil && *in.Index < 1 {
		return fail("index must be 1 or more")
	}
	if in.Type != nil && *in.Type != "" && !oneOfStr(*in.Type, []string{"landline", "mobile-lvn", "landline-toll-free"}) {
		return fail("type must be landline, mobile-lvn or landline-toll-free")
	}
	for _, ft := range in.Features {
		if !oneOfStr(ft, []string{"SMS", "VOICE", "MMS"}) {
			return fail("feature %q is not valid: use SMS, VOICE or MMS", ft)
		}
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetAvailableNumbersResult]{}, err
	}
	q := url.Values{"country": {in.Country}}
	setQ(q, "type", in.Type)
	setQ(q, "pattern", in.Pattern)
	if in.SearchPattern != nil {
		q.Set("search_pattern", strconv.Itoa(*in.SearchPattern))
	}
	if len(in.Features) > 0 {
		q.Set("features", strings.Join(in.Features, ","))
	}
	if in.Size != nil {
		q.Set("size", strconv.Itoa(*in.Size))
	}
	if in.Index != nil {
		q.Set("index", strconv.Itoa(*in.Index))
	}
	var page wireAvailableNumbers
	if err := c.restForm(ctx, http.MethodGet, "/number/search", q, nil, &page); err != nil {
		return fail("%v", err)
	}
	out := GetAvailableNumbersResult{Count: page.Count, Numbers: []AvailableNumber{}}
	for _, n := range page.Numbers {
		out.Numbers = append(out.Numbers, AvailableNumber{Country: n.Country, Msisdn: n.MSISDN, Type: n.Type, Cost: n.Cost, Features: nonNilStrings(n.Features)})
	}
	return infer.FunctionResponse[GetAvailableNumbersResult]{Output: out}, nil
}

func oneOfStr(v string, set []string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// getSubAccounts
// ---------------------------------------------------------------------------

// GetSubAccounts lists the subaccounts of the provider's primary account.
type GetSubAccounts struct{}

// GetSubAccountsArgs filters the list by exact name.
type GetSubAccountsArgs struct {
	Name *string `pulumi:"name,optional"`
}

func (a *GetSubAccountsArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "Only subaccounts with exactly this name.")
}

// GetSubAccountsResult lists subaccounts.
type GetSubAccountsResult struct {
	PrimaryAccountAPIKey string                `pulumi:"primaryAccountApiKey"`
	SubAccounts          []GetSubAccountResult `pulumi:"subAccounts"`
}

func (a *GetSubAccountsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.PrimaryAccountAPIKey, "API key of the primary account (the provider's own key).")
	an.Describe(&a.SubAccounts, "The matching subaccounts.")
}

func (f *GetSubAccounts) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the subaccounts of the provider's primary account, optionally filtered by exact name.")
}

type wireSubaccountList struct {
	Embedded struct {
		Subaccounts []wireSubaccount `json:"subaccounts"`
	} `json:"_embedded"`
}

func (f *GetSubAccounts) Invoke(ctx context.Context, req infer.FunctionRequest[GetSubAccountsArgs]) (infer.FunctionResponse[GetSubAccountsResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetSubAccountsResult]{}, err
	}
	var list wireSubaccountList
	if err := c.apiJSON(ctx, http.MethodGet, c.subPath(""), nil, nil, &list); err != nil {
		return infer.FunctionResponse[GetSubAccountsResult]{}, fmt.Errorf("getSubAccounts: %w", err)
	}
	out := GetSubAccountsResult{PrimaryAccountAPIKey: c.apiKey, SubAccounts: []GetSubAccountResult{}}
	for _, s := range list.Embedded.Subaccounts {
		if req.Input.Name != nil && *req.Input.Name != "" && s.Name != *req.Input.Name {
			continue
		}
		out.SubAccounts = append(out.SubAccounts, GetSubAccountResult{
			APIKey: s.APIKey, Name: s.Name, PrimaryAccountAPIKey: s.PrimaryAccountAPIKey,
			UsePrimaryAccountBalance: s.UsePrimaryAccountBalance, Suspended: s.Suspended, CreatedAt: s.CreatedAt,
			Balance: s.Balance, CreditLimit: s.CreditLimit,
		})
	}
	return infer.FunctionResponse[GetSubAccountsResult]{Output: out}, nil
}
