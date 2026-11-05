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

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// VerifyTemplateFragment manages the text of one Verify v2 template for one channel
// and locale.
type VerifyTemplateFragment struct{}

// VerifyTemplateFragmentArgs are the inputs of a VerifyTemplateFragment.
type VerifyTemplateFragmentArgs struct {
	TemplateID string `pulumi:"templateId" provider:"replaceOnChanges"`
	Channel    string `pulumi:"channel" provider:"replaceOnChanges"`
	Locale     string `pulumi:"locale" provider:"replaceOnChanges"`
	Text       string `pulumi:"text"`
}

func (a *VerifyTemplateFragmentArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.TemplateID, "ID of the VerifyTemplate this fragment belongs to. Changing it replaces the fragment.")
	an.Describe(&a.Channel, "Delivery channel: sms or voice. Changing it replaces the fragment.")
	an.Describe(&a.Locale, "Locale of the text, for example en-us. Changing it replaces the fragment.")
	an.Describe(&a.Text, "The message text, including Vonage placeholders such as ${code}. Updated in place.")
}

// VerifyTemplateFragmentState is the persisted state of a VerifyTemplateFragment.
type VerifyTemplateFragmentState struct {
	VerifyTemplateFragmentArgs

	// FragmentID is the server-assigned fragment ID.
	FragmentID string `pulumi:"templateFragmentId"`
	// DateCreated is the creation timestamp.
	DateCreated string `pulumi:"dateCreated"`
	// DateUpdated is the last update timestamp.
	DateUpdated string `pulumi:"dateUpdated"`
}

func (s *VerifyTemplateFragmentState) Annotate(a infer.Annotator) {
	a.Describe(&s.FragmentID, "The Vonage-assigned fragment ID. The resource ID is templateId/templateFragmentId.")
	a.Describe(&s.DateCreated, "Creation timestamp.")
	a.Describe(&s.DateUpdated, "Last update timestamp.")
}

func (r *VerifyTemplateFragment) Annotate(a infer.Annotator) {
	a.Describe(&r, "The text of a Verify v2 template for one channel and locale. Import with templateId/templateFragmentId.")
}

var _ infer.CustomResource[VerifyTemplateFragmentArgs, VerifyTemplateFragmentState] = (*VerifyTemplateFragment)(nil)
var _ infer.CustomCheck[VerifyTemplateFragmentArgs] = (*VerifyTemplateFragment)(nil)
var _ infer.CustomRead[VerifyTemplateFragmentArgs, VerifyTemplateFragmentState] = (*VerifyTemplateFragment)(nil)
var _ infer.CustomDiff[VerifyTemplateFragmentArgs, VerifyTemplateFragmentState] = (*VerifyTemplateFragment)(nil)
var _ infer.CustomUpdate[VerifyTemplateFragmentArgs, VerifyTemplateFragmentState] = (*VerifyTemplateFragment)(nil)
var _ infer.CustomDelete[VerifyTemplateFragmentState] = (*VerifyTemplateFragment)(nil)

func (r *VerifyTemplateFragment) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[VerifyTemplateFragmentArgs], error) {
	args, failures, err := infer.DefaultCheck[VerifyTemplateFragmentArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[VerifyTemplateFragmentArgs]{Inputs: args, Failures: failures}, err
	}
	if args.Channel != "sms" && args.Channel != "voice" {
		failures = append(failures, p.CheckFailure{Property: "channel", Reason: "channel must be sms or voice"})
	}
	if strings.TrimSpace(args.TemplateID) == "" {
		failures = append(failures, p.CheckFailure{Property: "templateId", Reason: "templateId must not be empty"})
	}
	if strings.TrimSpace(args.Locale) == "" {
		failures = append(failures, p.CheckFailure{Property: "locale", Reason: "locale must not be empty"})
	}
	if strings.TrimSpace(args.Text) == "" {
		failures = append(failures, p.CheckFailure{Property: "text", Reason: "text must not be empty"})
	}
	return infer.CheckResponse[VerifyTemplateFragmentArgs]{Inputs: args, Failures: failures}, nil
}

func fragmentID(templateID, id string) string { return templateID + "/" + id }

func fragmentState(a VerifyTemplateFragmentArgs, f *wireFragment) VerifyTemplateFragmentState {
	return VerifyTemplateFragmentState{VerifyTemplateFragmentArgs: a, FragmentID: f.TemplateFragmentID, DateCreated: f.DateCreated, DateUpdated: f.DateUpdated}
}

func (r *VerifyTemplateFragment) Create(ctx context.Context, req infer.CreateRequest[VerifyTemplateFragmentArgs]) (infer.CreateResponse[VerifyTemplateFragmentState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[VerifyTemplateFragmentState]{ID: req.Name, Output: VerifyTemplateFragmentState{VerifyTemplateFragmentArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[VerifyTemplateFragmentState]{}, err
	}
	f, err := c.CreateFragment(ctx, a.TemplateID, a.Channel, a.Locale, a.Text)
	if err != nil {
		return infer.CreateResponse[VerifyTemplateFragmentState]{}, fmt.Errorf("create Verify template fragment (%s, %s) in template %s: %w", a.Channel, a.Locale, a.TemplateID, err)
	}
	if f.TemplateFragmentID == "" {
		return infer.CreateResponse[VerifyTemplateFragmentState]{}, fmt.Errorf("create Verify template fragment: the response carried no template_fragment_id")
	}
	return infer.CreateResponse[VerifyTemplateFragmentState]{ID: fragmentID(a.TemplateID, f.TemplateFragmentID), Output: fragmentState(a, f)}, nil
}

func splitFragmentID(id string) (string, string, error) {
	t, f, ok := strings.Cut(id, "/")
	if !ok || t == "" || f == "" || strings.Contains(f, "/") {
		return "", "", fmt.Errorf("invalid VerifyTemplateFragment ID %q: expected TEMPLATE_ID/FRAGMENT_ID", id)
	}
	return t, f, nil
}

func (r *VerifyTemplateFragment) Read(ctx context.Context, req infer.ReadRequest[VerifyTemplateFragmentArgs, VerifyTemplateFragmentState]) (infer.ReadResponse[VerifyTemplateFragmentArgs, VerifyTemplateFragmentState], error) {
	type resp = infer.ReadResponse[VerifyTemplateFragmentArgs, VerifyTemplateFragmentState]
	tid, fid, err := splitFragmentID(req.ID)
	if err != nil {
		return resp{}, err
	}
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	f, err := c.GetFragment(ctx, tid, fid)
	if err != nil {
		return resp{}, fmt.Errorf("read Verify template fragment %s: %w", req.ID, err)
	}
	if f == nil {
		return resp{}, nil
	}
	args := VerifyTemplateFragmentArgs{TemplateID: tid, Channel: f.Channel, Locale: f.Locale, Text: f.Text}
	// Locale tags are case-insensitive: keep the declared spelling when the server only differs in case.
	if strings.EqualFold(req.Inputs.Locale, f.Locale) {
		args.Locale = req.Inputs.Locale
	}
	return resp{ID: req.ID, Inputs: args, State: fragmentState(args, f)}, nil
}

func (r *VerifyTemplateFragment) Diff(ctx context.Context, req infer.DiffRequest[VerifyTemplateFragmentArgs, VerifyTemplateFragmentState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.VerifyTemplateFragmentArgs
	if in.TemplateID != old.TemplateID {
		d["templateId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.Channel != old.Channel {
		d["channel"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if !strings.EqualFold(in.Locale, old.Locale) {
		d["locale"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.Text != old.Text {
		d["text"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *VerifyTemplateFragment) Update(ctx context.Context, req infer.UpdateRequest[VerifyTemplateFragmentArgs, VerifyTemplateFragmentState]) (infer.UpdateResponse[VerifyTemplateFragmentState], error) {
	a := req.Inputs
	st := req.State
	st.VerifyTemplateFragmentArgs = a
	if req.DryRun {
		return infer.UpdateResponse[VerifyTemplateFragmentState]{Output: st}, nil
	}
	tid, fid, err := splitFragmentID(req.ID)
	if err != nil {
		return infer.UpdateResponse[VerifyTemplateFragmentState]{}, err
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[VerifyTemplateFragmentState]{}, err
	}
	if err := c.PatchFragment(ctx, tid, fid, a.Text); err != nil {
		return infer.UpdateResponse[VerifyTemplateFragmentState]{}, fmt.Errorf("update Verify template fragment %s: %w", req.ID, err)
	}
	// Refresh the timestamps; the text is already applied, so a failed read-back is not fatal.
	if f, err := c.GetFragment(ctx, tid, fid); err == nil && f != nil {
		st = fragmentState(a, f)
	}
	return infer.UpdateResponse[VerifyTemplateFragmentState]{Output: st}, nil
}

func (r *VerifyTemplateFragment) Delete(ctx context.Context, req infer.DeleteRequest[VerifyTemplateFragmentState]) (infer.DeleteResponse, error) {
	tid, fid, err := splitFragmentID(req.ID)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.DeleteFragment(ctx, tid, fid); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete Verify template fragment %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// GetVerifyTemplateFragment looks up one Verify v2 template fragment, by fragment ID or
// by channel and locale.
type GetVerifyTemplateFragment struct{}

// GetVerifyTemplateFragmentArgs are the lookup arguments: templateId plus either
// templateFragmentId or both channel and locale.
type GetVerifyTemplateFragmentArgs struct {
	TemplateID         string  `pulumi:"templateId"`
	TemplateFragmentID *string `pulumi:"templateFragmentId,optional"`
	Channel            *string `pulumi:"channel,optional"`
	Locale             *string `pulumi:"locale,optional"`
}

func (a *GetVerifyTemplateFragmentArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.TemplateID, "The template the fragment belongs to.")
	an.Describe(&a.TemplateFragmentID, "Fragment ID. Set this, or both channel and locale.")
	an.Describe(&a.Channel, "Channel (sms or voice), used with locale.")
	an.Describe(&a.Locale, "Locale, used with channel.")
}

// GetVerifyTemplateFragmentResult is the looked-up fragment.
type GetVerifyTemplateFragmentResult struct {
	TemplateID         string `pulumi:"templateId"`
	TemplateFragmentID string `pulumi:"templateFragmentId"`
	Channel            string `pulumi:"channel"`
	Locale             string `pulumi:"locale"`
	Text               string `pulumi:"text"`
	DateCreated        string `pulumi:"dateCreated"`
	DateUpdated        string `pulumi:"dateUpdated"`
}

func (a *GetVerifyTemplateFragmentResult) Annotate(an infer.Annotator) {
	an.Describe(&a.TemplateID, "Template ID.")
	an.Describe(&a.TemplateFragmentID, "Fragment ID.")
	an.Describe(&a.Channel, "Channel.")
	an.Describe(&a.Locale, "Locale.")
	an.Describe(&a.Text, "Message text.")
	an.Describe(&a.DateCreated, "Creation timestamp.")
	an.Describe(&a.DateUpdated, "Last update timestamp.")
}

func (f *GetVerifyTemplateFragment) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Verify v2 template fragment by fragment ID, or by channel and locale.")
}

func (f *GetVerifyTemplateFragment) Invoke(ctx context.Context, req infer.FunctionRequest[GetVerifyTemplateFragmentArgs]) (infer.FunctionResponse[GetVerifyTemplateFragmentResult], error) {
	type out = infer.FunctionResponse[GetVerifyTemplateFragmentResult]
	in := req.Input
	hasID := in.TemplateFragmentID != nil && *in.TemplateFragmentID != ""
	hasCL := in.Channel != nil && *in.Channel != "" && in.Locale != nil && *in.Locale != ""
	if in.TemplateID == "" || hasID == hasCL {
		return out{}, fmt.Errorf("getVerifyTemplateFragment: set templateId and either templateFragmentId or both channel and locale")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return out{}, err
	}
	var frag *wireFragment
	if hasID {
		frag, err = c.GetFragment(ctx, in.TemplateID, *in.TemplateFragmentID)
		if err != nil {
			return out{}, fmt.Errorf("getVerifyTemplateFragment: %w", err)
		}
		if frag == nil {
			return out{}, fmt.Errorf("getVerifyTemplateFragment: fragment %q in template %q was not found", *in.TemplateFragmentID, in.TemplateID)
		}
	} else {
		all, err := c.ListFragments(ctx, in.TemplateID)
		if err != nil {
			return out{}, fmt.Errorf("getVerifyTemplateFragment: %w", err)
		}
		for i := range all {
			if all[i].Channel == *in.Channel && strings.EqualFold(all[i].Locale, *in.Locale) {
				frag = &all[i]
				break
			}
		}
		if frag == nil {
			return out{}, fmt.Errorf("getVerifyTemplateFragment: no %s fragment for locale %q in template %q was found", *in.Channel, *in.Locale, in.TemplateID)
		}
	}
	return out{Output: GetVerifyTemplateFragmentResult{TemplateID: in.TemplateID, TemplateFragmentID: frag.TemplateFragmentID,
		Channel: frag.Channel, Locale: frag.Locale, Text: frag.Text, DateCreated: frag.DateCreated, DateUpdated: frag.DateUpdated}}, nil
}
