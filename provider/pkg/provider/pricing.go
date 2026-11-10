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
	"regexp"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// getPricing: outbound SMS and voice prices
// ---------------------------------------------------------------------------

// GetPricing reads Vonage's outbound price list. It is read-only.
type GetPricing struct{}

// GetPricingArgs selects which prices to read. With neither country nor prefix the whole price list is read.
type GetPricingArgs struct {
	// ServiceType is sms, sms-transit or voice. Defaults to sms.
	ServiceType *string `pulumi:"serviceType,optional"`
	// Country restricts the result to one country (two-letter ISO code).
	Country *string `pulumi:"country,optional"`
	// Prefix restricts the result to the countries sharing a dialling prefix, such as 44.
	Prefix *string `pulumi:"prefix,optional"`
}

func (a *GetPricingArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.ServiceType, "The service to price: sms, sms-transit or voice. Defaults to sms.")
	an.Describe(&a.Country, "Two-letter ISO country code, for example GB. Cannot be combined with prefix. "+
		"With neither country nor prefix every country is returned.")
	an.Describe(&a.Prefix, "International dialling prefix, digits only, for example 44. Cannot be combined with country.")
}

// PricingNetwork is the price on one mobile network.
type PricingNetwork struct {
	Type        string `pulumi:"type"`
	Price       string `pulumi:"price"`
	Currency    string `pulumi:"currency"`
	Mcc         string `pulumi:"mcc"`
	Mnc         string `pulumi:"mnc"`
	NetworkCode string `pulumi:"networkCode"`
	NetworkName string `pulumi:"networkName"`
}

func (a *PricingNetwork) Annotate(an infer.Annotator) {
	an.Describe(&a.Type, "The type of network, such as mobile or landline.")
	an.Describe(&a.Price, "The price on this network, as a decimal string in the account currency.")
	an.Describe(&a.Currency, "The currency of the price.")
	an.Describe(&a.Mcc, "The Mobile Country Code of the operator.")
	an.Describe(&a.Mnc, "The Mobile Network Code of the operator.")
	an.Describe(&a.NetworkCode, "The MCC and MNC combined, a unique reference for the operator.")
	an.Describe(&a.NetworkName, "The name of the operator.")
}

// CountryPricing is the price list of one country.
type CountryPricing struct {
	CountryCode        string           `pulumi:"countryCode"`
	CountryName        string           `pulumi:"countryName"`
	CountryDisplayName string           `pulumi:"countryDisplayName"`
	Currency           string           `pulumi:"currency"`
	DefaultPrice       string           `pulumi:"defaultPrice"`
	DialingPrefix      string           `pulumi:"dialingPrefix"`
	Networks           []PricingNetwork `pulumi:"networks"`
}

func (a *CountryPricing) Annotate(an infer.Annotator) {
	an.Describe(&a.CountryCode, "Two-letter country code.")
	an.Describe(&a.CountryName, "The country name.")
	an.Describe(&a.CountryDisplayName, "The country name for display.")
	an.Describe(&a.Currency, "The currency the account is billed in.")
	an.Describe(&a.DefaultPrice, "The default price in this country, as a decimal string.")
	an.Describe(&a.DialingPrefix, "The dialling prefix of the country.")
	an.Describe(&a.Networks, "Prices per mobile network in the country.")
}

// GetPricingResult is the price list.
type GetPricingResult struct {
	ServiceType string           `pulumi:"serviceType"`
	Count       int              `pulumi:"count"`
	Countries   []CountryPricing `pulumi:"countries"`
}

func (a *GetPricingResult) Annotate(an infer.Annotator) {
	an.Describe(&a.ServiceType, "The service that was priced.")
	an.Describe(&a.Count, "Number of countries returned.")
	an.Describe(&a.Countries, "The price lists. One entry for a country lookup, one per matching country otherwise.")
}

func (f *GetPricing) Annotate(a infer.Annotator) {
	a.Describe(&f, "Read Vonage's outbound price list for SMS, SMS transit or voice, for one country (GET /account/get-pricing/outbound), "+
		"for a dialling prefix (GET /account/get-prefix-pricing/outbound) or for every country (GET /account/get-full-pricing/outbound). Read-only. "+
		"Prices are decimal strings in the account currency.")
}

type wirePricingNetwork struct {
	Type        string `json:"type"`
	Price       string `json:"price"`
	Currency    string `json:"currency"`
	Mcc         string `json:"mcc"`
	Mnc         string `json:"mnc"`
	NetworkCode string `json:"networkCode"`
	NetworkName string `json:"networkName"`
}

type wireCountryPricing struct {
	CountryCode        string               `json:"countryCode"`
	CountryName        string               `json:"countryName"`
	CountryDisplayName string               `json:"countryDisplayName"`
	Currency           string               `json:"currency"`
	DefaultPrice       string               `json:"defaultPrice"`
	DialingPrefix      string               `json:"dialingPrefix"`
	Networks           []wirePricingNetwork `json:"networks"`
}

type wireAllPricing struct {
	Count     *int                 `json:"count"`
	Countries []wireCountryPricing `json:"countries"`
}

func (w wireCountryPricing) view() CountryPricing {
	out := CountryPricing{CountryCode: w.CountryCode, CountryName: w.CountryName, CountryDisplayName: w.CountryDisplayName,
		Currency: w.Currency, DefaultPrice: w.DefaultPrice, DialingPrefix: w.DialingPrefix, Networks: []PricingNetwork{}}
	for _, n := range w.Networks {
		out.Networks = append(out.Networks, PricingNetwork(n))
	}
	return out
}

var pricingPrefix = regexp.MustCompile(`^[0-9]{1,6}$`)

func (f *GetPricing) Invoke(ctx context.Context, req infer.FunctionRequest[GetPricingArgs]) (infer.FunctionResponse[GetPricingResult], error) {
	in := req.Input
	fail := func(format string, a ...any) (infer.FunctionResponse[GetPricingResult], error) {
		return infer.FunctionResponse[GetPricingResult]{}, fmt.Errorf("getPricing: "+format, a...)
	}
	service := derefOrStr(in.ServiceType, "sms")
	if !oneOfStr(service, []string{"sms", "sms-transit", "voice"}) {
		return fail("serviceType must be sms, sms-transit or voice")
	}
	hasCountry, hasPrefix := in.Country != nil && *in.Country != "", in.Prefix != nil && *in.Prefix != ""
	if hasCountry && hasPrefix {
		return fail("set either country or prefix, not both")
	}
	if hasCountry && len(*in.Country) != 2 {
		return fail("country must be a two-letter ISO country code")
	}
	if hasPrefix && !pricingPrefix.MatchString(*in.Prefix) {
		return fail("prefix must be digits only, for example 44")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetPricingResult]{}, err
	}
	out := GetPricingResult{ServiceType: service, Countries: []CountryPricing{}}
	switch {
	case hasCountry:
		var one wireCountryPricing
		q := url.Values{"country": {*in.Country}}
		if err := c.restForm(ctx, http.MethodGet, "/account/get-pricing/outbound/"+service, q, nil, &one); err != nil {
			return fail("%v", err)
		}
		if one.CountryCode == "" {
			return fail("Vonage returned no pricing for country %q", *in.Country)
		}
		out.Countries = append(out.Countries, one.view())
	default:
		path, q := "/account/get-full-pricing/outbound/"+service, url.Values(nil)
		if hasPrefix {
			path, q = "/account/get-prefix-pricing/outbound/"+service, url.Values{"prefix": {*in.Prefix}}
		}
		var all wireAllPricing
		if err := c.restForm(ctx, http.MethodGet, path, q, nil, &all); err != nil {
			return fail("%v", err)
		}
		// A count above what was delivered means the answer is cut short: fail instead of reporting a partial price list.
		if all.Count != nil && *all.Count != len(all.Countries) {
			return fail("Vonage reported %d countries but returned %d", *all.Count, len(all.Countries))
		}
		for _, w := range all.Countries {
			out.Countries = append(out.Countries, w.view())
		}
	}
	out.Count = len(out.Countries)
	return infer.FunctionResponse[GetPricingResult]{Output: out}, nil
}

func derefOrStr(p *string, def string) string {
	if p == nil || *p == "" {
		return def
	}
	return *p
}
