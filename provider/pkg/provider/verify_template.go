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

// VerifyTemplate manages a Verify v2 template, the container for per-channel and
// per-locale message text. Text itself lives in VerifyTemplateFragment resources.
type VerifyTemplate struct{}

// VerifyTemplateArgs are the inputs of a VerifyTemplate.
type VerifyTemplateArgs struct {
	Name      string `pulumi:"name"`
	IsDefault *bool  `pulumi:"isDefault,optional"`
}

func (a *VerifyTemplateArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "Template name.")
	an.Describe(&a.IsDefault, "Whether this is the account's default template for Verify requests that name none (server default false). "+
		"Making another template the default un-defaults this one outside Pulumi, which shows as drift.")
}

// VerifyTemplateState is the persisted state of a VerifyTemplate.
type VerifyTemplateState struct {
	VerifyTemplateArgs

	// TemplateID is the server-assigned template ID and the resource ID.
	TemplateID string `pulumi:"templateId"`
}

func (s *VerifyTemplateState) Annotate(a infer.Annotator) {
	a.Describe(&s.TemplateID, "The Vonage-assigned template ID (also the resource ID).")
}

func (r *VerifyTemplate) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Verify v2 template. Import with the template ID.")
}

var _ infer.CustomResource[VerifyTemplateArgs, VerifyTemplateState] = (*VerifyTemplate)(nil)
var _ infer.CustomCheck[VerifyTemplateArgs] = (*VerifyTemplate)(nil)
var _ infer.CustomRead[VerifyTemplateArgs, VerifyTemplateState] = (*VerifyTemplate)(nil)
var _ infer.CustomDiff[VerifyTemplateArgs, VerifyTemplateState] = (*VerifyTemplate)(nil)
var _ infer.CustomUpdate[VerifyTemplateArgs, VerifyTemplateState] = (*VerifyTemplate)(nil)
var _ infer.CustomDelete[VerifyTemplateState] = (*VerifyTemplate)(nil)

func (r *VerifyTemplate) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[VerifyTemplateArgs], error) {
	args, failures, err := infer.DefaultCheck[VerifyTemplateArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[VerifyTemplateArgs]{Inputs: args, Failures: failures}, err
	}
	if utf8.RuneCountInString(strings.TrimSpace(args.Name)) < 1 {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	return infer.CheckResponse[VerifyTemplateArgs]{Inputs: args, Failures: failures}, nil
}

func (r *VerifyTemplate) Create(ctx context.Context, req infer.CreateRequest[VerifyTemplateArgs]) (infer.CreateResponse[VerifyTemplateState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[VerifyTemplateState]{ID: "", Output: VerifyTemplateState{VerifyTemplateArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[VerifyTemplateState]{}, err
	}
	t, err := c.CreateTemplate(ctx, a.Name)
	if err != nil {
		return infer.CreateResponse[VerifyTemplateState]{}, fmt.Errorf("create Verify template %q: %w", a.Name, err)
	}
	if t.TemplateID == "" {
		return infer.CreateResponse[VerifyTemplateState]{}, fmt.Errorf("create Verify template %q: the response carried no template_id", a.Name)
	}
	st := VerifyTemplateState{VerifyTemplateArgs: a, TemplateID: t.TemplateID}
	// The create call only takes the name; making the template the default is a PATCH.
	if effective(a.IsDefault, false) != t.IsDefault {
		if _, err := c.PatchTemplate(ctx, t.TemplateID, a.Name, effective(a.IsDefault, false)); err != nil {
			// Record what the server actually has, so the next Diff sees isDefault as a
			// pending change and Update retries the PATCH.
			actual := t.IsDefault
			st.IsDefault = &actual
			return infer.CreateResponse[VerifyTemplateState]{ID: t.TemplateID, Output: st},
				infer.ResourceInitFailedError{Reasons: []string{fmt.Sprintf("Verify template created but setting isDefault failed: %v", err)}}
		}
	}
	return infer.CreateResponse[VerifyTemplateState]{ID: t.TemplateID, Output: st}, nil
}

func (r *VerifyTemplate) Read(ctx context.Context, req infer.ReadRequest[VerifyTemplateArgs, VerifyTemplateState]) (infer.ReadResponse[VerifyTemplateArgs, VerifyTemplateState], error) {
	if req.ID == "" {
		return infer.ReadResponse[VerifyTemplateArgs, VerifyTemplateState]{}, fmt.Errorf("invalid VerifyTemplate ID: expected the template ID")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.ReadResponse[VerifyTemplateArgs, VerifyTemplateState]{}, err
	}
	t, err := c.GetTemplate(ctx, req.ID)
	if err != nil {
		return infer.ReadResponse[VerifyTemplateArgs, VerifyTemplateState]{}, fmt.Errorf("read Verify template %s: %w", req.ID, err)
	}
	if t == nil {
		if err := notFoundOnImport("Verify template", req.ID, req.Inputs, req.State); err != nil {
			return infer.ReadResponse[VerifyTemplateArgs, VerifyTemplateState]{}, err
		}
		return infer.ReadResponse[VerifyTemplateArgs, VerifyTemplateState]{}, nil
	}
	args := req.Inputs
	args.Name = t.Name
	// Unset means the server default (false): keep it unset while the server agrees, and
	// surface a true value so a template made default outside Pulumi shows as drift.
	if args.IsDefault != nil || t.IsDefault {
		v := t.IsDefault
		args.IsDefault = &v
	}
	return infer.ReadResponse[VerifyTemplateArgs, VerifyTemplateState]{ID: req.ID, Inputs: args,
		State: VerifyTemplateState{VerifyTemplateArgs: args, TemplateID: t.TemplateID}}, nil
}

func (r *VerifyTemplate) Diff(ctx context.Context, req infer.DiffRequest[VerifyTemplateArgs, VerifyTemplateState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.VerifyTemplateArgs
	if in.Name != old.Name {
		d["name"] = p.PropertyDiff{Kind: p.Update}
	}
	if effective(in.IsDefault, false) != effective(old.IsDefault, false) {
		d["isDefault"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *VerifyTemplate) Update(ctx context.Context, req infer.UpdateRequest[VerifyTemplateArgs, VerifyTemplateState]) (infer.UpdateResponse[VerifyTemplateState], error) {
	a := req.Inputs
	st := req.State
	st.VerifyTemplateArgs = a
	if req.DryRun {
		return infer.UpdateResponse[VerifyTemplateState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[VerifyTemplateState]{}, err
	}
	if _, err := c.PatchTemplate(ctx, req.ID, a.Name, effective(a.IsDefault, false)); err != nil {
		return infer.UpdateResponse[VerifyTemplateState]{}, fmt.Errorf("update Verify template %s: %w", req.ID, err)
	}
	return infer.UpdateResponse[VerifyTemplateState]{Output: st}, nil
}

func (r *VerifyTemplate) Delete(ctx context.Context, req infer.DeleteRequest[VerifyTemplateState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.DeleteTemplate(ctx, req.ID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete Verify template %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// GetVerifyTemplate looks up a Verify v2 template by ID or by exact name.
type GetVerifyTemplate struct{}

// GetVerifyTemplateArgs are the lookup arguments; set exactly one of templateId or name.
type GetVerifyTemplateArgs struct {
	TemplateID *string `pulumi:"templateId,optional"`
	Name       *string `pulumi:"name,optional"`
}

func (a *GetVerifyTemplateArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.TemplateID, "Template ID. Exactly one of templateId or name must be set.")
	an.Describe(&a.Name, "Exact template name. Fails if zero or more than one template matches.")
}

// GetVerifyTemplateResult is the looked-up template.
type GetVerifyTemplateResult struct {
	TemplateID string `pulumi:"templateId"`
	Name       string `pulumi:"name"`
	IsDefault  bool   `pulumi:"isDefault"`
}

func (a *GetVerifyTemplateResult) Annotate(an infer.Annotator) {
	an.Describe(&a.TemplateID, "Template ID.")
	an.Describe(&a.Name, "Template name.")
	an.Describe(&a.IsDefault, "Whether this is the account's default template.")
}

func (f *GetVerifyTemplate) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Verify v2 template by ID or exact name.")
}

func (f *GetVerifyTemplate) Invoke(ctx context.Context, req infer.FunctionRequest[GetVerifyTemplateArgs]) (infer.FunctionResponse[GetVerifyTemplateResult], error) {
	in := req.Input
	hasID, hasName := in.TemplateID != nil && *in.TemplateID != "", in.Name != nil && *in.Name != ""
	if hasID == hasName {
		return infer.FunctionResponse[GetVerifyTemplateResult]{}, fmt.Errorf("getVerifyTemplate: set exactly one of templateId or name")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetVerifyTemplateResult]{}, err
	}
	var t *wireTemplate
	if hasID {
		t, err = c.GetTemplate(ctx, *in.TemplateID)
		if err != nil {
			return infer.FunctionResponse[GetVerifyTemplateResult]{}, fmt.Errorf("getVerifyTemplate: %w", err)
		}
		if t == nil {
			return infer.FunctionResponse[GetVerifyTemplateResult]{}, fmt.Errorf("getVerifyTemplate: no Verify template with id %q was found", *in.TemplateID)
		}
	} else {
		all, err := c.ListTemplates(ctx)
		if err != nil {
			return infer.FunctionResponse[GetVerifyTemplateResult]{}, fmt.Errorf("getVerifyTemplate: %w", err)
		}
		var matches []wireTemplate
		for _, x := range all {
			if x.Name == *in.Name {
				matches = append(matches, x)
			}
		}
		switch len(matches) {
		case 0:
			return infer.FunctionResponse[GetVerifyTemplateResult]{}, fmt.Errorf("getVerifyTemplate: no Verify template named %q was found", *in.Name)
		case 1:
			t = &matches[0]
		default:
			return infer.FunctionResponse[GetVerifyTemplateResult]{}, fmt.Errorf("getVerifyTemplate: %d Verify templates are named %q; look up by templateId", len(matches), *in.Name)
		}
	}
	return infer.FunctionResponse[GetVerifyTemplateResult]{Output: GetVerifyTemplateResult{TemplateID: t.TemplateID, Name: t.Name, IsDefault: t.IsDefault}}, nil
}
