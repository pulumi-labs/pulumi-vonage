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

	"github.com/pulumi/pulumi-go-provider/infer"
)

// GetApplication looks up a Vonage application by ID or by exact name.
type GetApplication struct{}

// GetApplicationArgs are the lookup arguments; set exactly one of applicationId or name.
type GetApplicationArgs struct {
	ApplicationID *string `pulumi:"applicationId,optional"`
	Name          *string `pulumi:"name,optional"`
}

func (a *GetApplicationArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.ApplicationID, "Application ID (UUID). Exactly one of applicationId or name must be set.")
	an.Describe(&a.Name, "Exact application name. Fails if zero or more than one application matches.")
}

// GetApplicationResult is the looked-up application.
type GetApplicationResult struct {
	ApplicationID string   `pulumi:"applicationId"`
	Name          string   `pulumi:"name"`
	PublicKey     string   `pulumi:"publicKey"`
	Capabilities  []string `pulumi:"capabilities"`
}

func (a *GetApplicationResult) Annotate(an infer.Annotator) {
	an.Describe(&a.ApplicationID, "Application ID.")
	an.Describe(&a.Name, "Application name.")
	an.Describe(&a.PublicKey, "PEM public key registered with the application.")
	an.Describe(&a.Capabilities, "Enabled capabilities, sorted: messages, meetings, rtc, vbc, verify, voice.")
}

func (f *GetApplication) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Vonage application by ID or exact name.")
}

func capabilityNames(c *wireCapabilities) []string {
	out := []string{}
	if c == nil {
		return out
	}
	if c.Messages != nil {
		out = append(out, "messages")
	}
	if c.Meetings != nil {
		out = append(out, "meetings")
	}
	if c.RTC != nil {
		out = append(out, "rtc")
	}
	if c.VBC != nil {
		out = append(out, "vbc")
	}
	if c.Verify != nil {
		out = append(out, "verify")
	}
	if c.Voice != nil {
		out = append(out, "voice")
	}
	return out
}

func (f *GetApplication) Invoke(ctx context.Context, req infer.FunctionRequest[GetApplicationArgs]) (infer.FunctionResponse[GetApplicationResult], error) {
	in := req.Input
	hasID, hasName := in.ApplicationID != nil && *in.ApplicationID != "", in.Name != nil && *in.Name != ""
	if hasID == hasName {
		return infer.FunctionResponse[GetApplicationResult]{}, fmt.Errorf("getApplication: set exactly one of applicationId or name")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetApplicationResult]{}, err
	}
	var app *wireApplication
	if hasID {
		app, err = c.GetApplication(ctx, *in.ApplicationID)
		if err != nil {
			return infer.FunctionResponse[GetApplicationResult]{}, fmt.Errorf("getApplication: %w", err)
		}
		if app == nil {
			return infer.FunctionResponse[GetApplicationResult]{}, fmt.Errorf("getApplication: no application with id %q", *in.ApplicationID)
		}
	} else {
		apps, err := c.ListApplications(ctx)
		if err != nil {
			return infer.FunctionResponse[GetApplicationResult]{}, fmt.Errorf("getApplication: %w", err)
		}
		var matches []wireApplication
		for _, a := range apps {
			if a.Name == *in.Name {
				matches = append(matches, a)
			}
		}
		switch len(matches) {
		case 0:
			return infer.FunctionResponse[GetApplicationResult]{}, fmt.Errorf("getApplication: no application named %q", *in.Name)
		case 1:
			app = &matches[0]
		default:
			return infer.FunctionResponse[GetApplicationResult]{}, fmt.Errorf("getApplication: %d applications are named %q; look up by applicationId", len(matches), *in.Name)
		}
	}
	res := GetApplicationResult{ApplicationID: app.ID, Name: app.Name, Capabilities: capabilityNames(app.Capabilities)}
	if app.Keys != nil {
		res.PublicKey = app.Keys.PublicKey
	}
	return infer.FunctionResponse[GetApplicationResult]{Output: res}, nil
}

// GetNumber looks up an owned Vonage number.
type GetNumber struct{}

// GetNumberArgs are the lookup arguments.
type GetNumberArgs struct {
	Country string `pulumi:"country"`
	Msisdn  string `pulumi:"msisdn"`
}

func (a *GetNumberArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Country, "Two-letter ISO country code, for example US.")
	an.Describe(&a.Msisdn, "The number in international format without a leading plus.")
}

// GetNumberResult is an owned number's configuration.
type GetNumberResult struct {
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

func (a *GetNumberResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Country, "Country code.")
	an.Describe(&a.Msisdn, "The number.")
	an.Describe(&a.Type, "Number type (landline, mobile-lvn, landline-toll-free).")
	an.Describe(&a.Features, "Capabilities: SMS, VOICE, MMS.")
	an.Describe(&a.AppID, "Linked application ID, empty if none.")
	an.Describe(&a.MoHTTPURL, "Inbound SMS webhook, empty if none.")
	an.Describe(&a.VoiceCallbackType, "Voice routing type.")
	an.Describe(&a.VoiceCallbackValue, "Voice routing target.")
	an.Describe(&a.MessagesCallbackType, "Messages routing type.")
	an.Describe(&a.MessagesCallbackValue, "Messages routing target.")
}

func (f *GetNumber) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a number owned by the account. Fails if the account does not own it.")
}

func (f *GetNumber) Invoke(ctx context.Context, req infer.FunctionRequest[GetNumberArgs]) (infer.FunctionResponse[GetNumberResult], error) {
	in := req.Input
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNumberResult]{}, err
	}
	n, err := c.FindOwnedNumber(ctx, in.Country, in.Msisdn)
	if err != nil {
		return infer.FunctionResponse[GetNumberResult]{}, fmt.Errorf("getNumber: %w", err)
	}
	if n == nil {
		return infer.FunctionResponse[GetNumberResult]{}, fmt.Errorf("getNumber: %s (%s) is not owned by this account", in.Msisdn, in.Country)
	}
	feats := n.Features
	if feats == nil {
		feats = []string{}
	}
	return infer.FunctionResponse[GetNumberResult]{Output: GetNumberResult{
		Country: in.Country, Msisdn: in.Msisdn, Type: n.Type, Features: feats, AppID: n.AppID, MoHTTPURL: n.MoHTTPURL,
		VoiceCallbackType: n.VoiceCallbackType, VoiceCallbackValue: n.VoiceCallbackValue,
		MessagesCallbackType: n.MessagesCallbackType, MessagesCallbackValue: n.MessagesCallbackValue,
	}}, nil
}

// GetApplications lists the account's applications, optionally filtered by exact name.
type GetApplications struct{}

// GetApplicationsArgs are the lookup arguments.
type GetApplicationsArgs struct {
	Name *string `pulumi:"name,optional"`
}

func (a *GetApplicationsArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "Only return applications with exactly this name.")
}

// ApplicationSummary is one listed application.
type ApplicationSummary struct {
	ApplicationID string   `pulumi:"applicationId"`
	Name          string   `pulumi:"name"`
	Capabilities  []string `pulumi:"capabilities"`
}

func (a *ApplicationSummary) Annotate(an infer.Annotator) {
	an.Describe(&a.ApplicationID, "Application ID.")
	an.Describe(&a.Name, "Application name.")
	an.Describe(&a.Capabilities, "Enabled capabilities, sorted.")
}

// GetApplicationsResult lists applications.
type GetApplicationsResult struct {
	Applications []ApplicationSummary `pulumi:"applications"`
}

func (a *GetApplicationsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Applications, "The matching applications.")
}

func (f *GetApplications) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the account's Vonage applications, optionally filtered by exact name.")
}

func (f *GetApplications) Invoke(ctx context.Context, req infer.FunctionRequest[GetApplicationsArgs]) (infer.FunctionResponse[GetApplicationsResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetApplicationsResult]{}, err
	}
	apps, err := c.ListApplications(ctx)
	if err != nil {
		return infer.FunctionResponse[GetApplicationsResult]{}, fmt.Errorf("getApplications: %w", err)
	}
	out := GetApplicationsResult{Applications: []ApplicationSummary{}}
	for _, a := range apps {
		if req.Input.Name != nil && *req.Input.Name != "" && a.Name != *req.Input.Name {
			continue
		}
		out.Applications = append(out.Applications, ApplicationSummary{ApplicationID: a.ID, Name: a.Name, Capabilities: capabilityNames(a.Capabilities)})
	}
	return infer.FunctionResponse[GetApplicationsResult]{Output: out}, nil
}
