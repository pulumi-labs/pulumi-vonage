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
	"strconv"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// Number Pools API (https://developer.vonage.com/en/api/number-pools, spec v2.0.4).
// The request and response shapes below follow the published OpenAPI document and
// have not been exercised against a live Vonage account. The pool account ID is the
// account's API key.

const (
	poolPageSize = 100
	// maxPoolPages bounds the pages a pool listing follows.
	maxPoolPages = 100
)

var (
	poolFallbacks = []string{"Pool", "CustomFrom", "Reject"}
	poolNumberRE  = regexp.MustCompile(`^[0-9]{7,15}$`)
)

type wirePool struct {
	PoolID                     string `json:"pool_id"`
	AccountID                  string `json:"account_id,omitempty"`
	Fallback                   string `json:"fallback"`
	LocalizedSenderPreferred   bool   `json:"localized_sender_preferred"`
	PredictableSenderPreferred bool   `json:"predictable_sender_preferred"`
}

type wirePoolPatch struct {
	Fallback                   *string `json:"fallback,omitempty"`
	LocalizedSenderPreferred   *bool   `json:"localized_sender_preferred,omitempty"`
	PredictableSenderPreferred *bool   `json:"predictable_sender_preferred,omitempty"`
}

// wirePoolPage is the paging envelope shared by the pool and pool-number listings.
type wirePoolPage struct {
	TotalPages *int `json:"total_pages"`
	Embedded   struct {
		Pools   []wirePool `json:"pools"`
		Numbers []string   `json:"numbers"`
	} `json:"_embedded"`
}

func (c *Client) poolBase() string {
	return "/v2/numberpools/accounts/" + url.PathEscape(c.apiKey) + "/pools"
}

func (c *Client) poolPath(poolID string) string {
	return c.poolBase() + "/" + url.PathEscape(poolID)
}

// CreatePool creates a pool.
func (c *Client) CreatePool(ctx context.Context, in wirePool) (*wirePool, error) {
	var out wirePool
	if err := c.apiJSON(ctx, http.MethodPost, c.poolBase(), nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPool returns nil, nil when the pool does not exist.
func (c *Client) GetPool(ctx context.Context, poolID string) (*wirePool, error) {
	var out wirePool
	err := c.apiJSON(ctx, http.MethodGet, c.poolPath(poolID), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PatchPool updates a pool.
func (c *Client) PatchPool(ctx context.Context, poolID string, in wirePoolPatch) (*wirePool, error) {
	var out wirePool
	if err := c.apiJSON(ctx, http.MethodPatch, c.poolPath(poolID), nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePool deletes a pool; a missing pool is not an error.
func (c *Client) DeletePool(ctx context.Context, poolID string) error {
	err := c.apiJSON(ctx, http.MethodDelete, c.poolPath(poolID), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// walkPoolPages follows every page of a listing. It fails loudly when the server does
// not say how many pages exist (rather than guessing) or when the page cap is hit.
func (c *Client) walkPoolPages(ctx context.Context, path string, each func(wirePoolPage)) error {
	for page := 1; page <= maxPoolPages; page++ {
		q := url.Values{"page_size": {strconv.Itoa(poolPageSize)}, "page": {strconv.Itoa(page)}}
		var pg wirePoolPage
		if err := c.apiJSON(ctx, http.MethodGet, path, q, nil, &pg); err != nil {
			return err
		}
		each(pg)
		if pg.TotalPages == nil {
			if page == 1 && len(pg.Embedded.Pools) == 0 && len(pg.Embedded.Numbers) == 0 {
				return nil
			}
			return fmt.Errorf("vonage: GET %s page %d: response has no total_pages, refusing to guess whether more pages exist", path, page)
		}
		if page >= *pg.TotalPages {
			return nil
		}
	}
	return fmt.Errorf("vonage: GET %s: more than %d pages", path, maxPoolPages)
}

// ListPools returns every pool of the account.
func (c *Client) ListPools(ctx context.Context) ([]wirePool, error) {
	out := []wirePool{}
	err := c.walkPoolPages(ctx, c.poolBase(), func(pg wirePoolPage) { out = append(out, pg.Embedded.Pools...) })
	return out, err
}

// ListPoolNumbers returns every number in a pool. found is false when the pool does not exist.
func (c *Client) ListPoolNumbers(ctx context.Context, poolID string) (nums []string, found bool, err error) {
	nums = []string{}
	err = c.walkPoolPages(ctx, c.poolPath(poolID)+"/numbers", func(pg wirePoolPage) { nums = append(nums, pg.Embedded.Numbers...) })
	if isNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return nums, true, nil
}

// AddPoolNumber adds one owned number to a pool.
func (c *Client) AddPoolNumber(ctx context.Context, poolID, number string) error {
	var out struct {
		Errors []string `json:"errors"`
	}
	body := map[string][]string{"numbers": {number}}
	if err := c.apiJSON(ctx, http.MethodPost, c.poolPath(poolID)+"/numbers", nil, body, &out); err != nil {
		return err
	}
	if len(out.Errors) > 0 {
		return fmt.Errorf("vonage: number(s) %s could not be added to pool %q", strings.Join(out.Errors, ", "), poolID)
	}
	return nil
}

// RemovePoolNumber removes a number from a pool; a missing pool or membership is not an error.
func (c *Client) RemovePoolNumber(ctx context.Context, poolID, number string) error {
	err := c.apiJSON(ctx, http.MethodDelete, c.poolPath(poolID)+"/numbers/"+url.PathEscape(number), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

func validFallback(s string) bool {
	for _, f := range poolFallbacks {
		if s == f {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// NumberPool
// ---------------------------------------------------------------------------

// NumberPool manages a Vonage number pool: a named group of owned numbers that
// Vonage picks a sender from when you message without a fixed sender.
type NumberPool struct{}

// NumberPoolArgs are the inputs of a NumberPool.
type NumberPoolArgs struct {
	PoolID                     string `pulumi:"poolId"`
	Fallback                   string `pulumi:"fallback"`
	LocalizedSenderPreferred   *bool  `pulumi:"localizedSenderPreferred,optional"`
	PredictableSenderPreferred *bool  `pulumi:"predictableSenderPreferred,optional"`
}

func (a *NumberPoolArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.PoolID, "Your identifier for the pool, unique within the account. Also the resource ID. Changing it replaces the pool.")
	an.Describe(&a.Fallback, "What Vonage does when no number in the pool can send the message: Pool (use any pool number), \"CustomFrom\" (use the sender in the request) or Reject (fail the message).")
	an.Describe(&a.LocalizedSenderPreferred, "Prefer a pool number local to the destination country when one exists. Defaults to false when unset (Vonage requires the flag on create, so false is sent).")
	an.Describe(&a.PredictableSenderPreferred, "Keep using the same pool number for the same recipient when possible. Defaults to false when unset (Vonage requires the flag on create, so false is sent).")
}

// NumberPoolState is the persisted state of a NumberPool.
type NumberPoolState struct {
	NumberPoolArgs

	// AccountID is the account (API key) that owns the pool.
	AccountID string `pulumi:"accountId"`
}

func (s *NumberPoolState) Annotate(a infer.Annotator) {
	a.Describe(&s.AccountID, "ID of the account that owns the pool (the API key).")
}

func (r *NumberPool) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Vonage number pool. Add numbers with NumberPoolNumber. Shapes follow the published Number Pools API specification and were not verified against a live account.")
}

var _ infer.CustomResource[NumberPoolArgs, NumberPoolState] = (*NumberPool)(nil)
var _ infer.CustomCheck[NumberPoolArgs] = (*NumberPool)(nil)
var _ infer.CustomRead[NumberPoolArgs, NumberPoolState] = (*NumberPool)(nil)
var _ infer.CustomUpdate[NumberPoolArgs, NumberPoolState] = (*NumberPool)(nil)
var _ infer.CustomDiff[NumberPoolArgs, NumberPoolState] = (*NumberPool)(nil)
var _ infer.CustomDelete[NumberPoolState] = (*NumberPool)(nil)

func (r *NumberPool) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[NumberPoolArgs], error) {
	args, failures, err := infer.DefaultCheck[NumberPoolArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[NumberPoolArgs]{Inputs: args, Failures: failures}, err
	}
	if strings.TrimSpace(args.PoolID) == "" || strings.Contains(args.PoolID, "/") {
		failures = append(failures, p.CheckFailure{Property: "poolId", Reason: "poolId must be non-empty and must not contain \"/\""})
	}
	if !validFallback(args.Fallback) {
		failures = append(failures, p.CheckFailure{Property: "fallback", Reason: "fallback must be one of Pool, CustomFrom, Reject"})
	}
	return infer.CheckResponse[NumberPoolArgs]{Inputs: args, Failures: failures}, nil
}

// keepOptional reports a server flag as an input: unset stays unset while the server agrees
// with the default (false, which is also what an absent flag reads as), anything else is surfaced
// so an import adopts it and drift shows.
func keepOptional(in *bool, server bool) *bool {
	if in != nil || server {
		v := server
		return &v
	}
	return nil
}

func poolStateFrom(w *wirePool, accountFallback string, prior NumberPoolArgs) NumberPoolState {
	acct := w.AccountID
	if acct == "" {
		acct = accountFallback
	}
	return NumberPoolState{
		NumberPoolArgs: NumberPoolArgs{PoolID: w.PoolID, Fallback: w.Fallback,
			LocalizedSenderPreferred:   keepOptional(prior.LocalizedSenderPreferred, w.LocalizedSenderPreferred),
			PredictableSenderPreferred: keepOptional(prior.PredictableSenderPreferred, w.PredictableSenderPreferred)},
		AccountID: acct,
	}
}

func (r *NumberPool) Create(ctx context.Context, req infer.CreateRequest[NumberPoolArgs]) (infer.CreateResponse[NumberPoolState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[NumberPoolState]{Output: NumberPoolState{NumberPoolArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[NumberPoolState]{}, err
	}
	w, err := c.CreatePool(ctx, wirePool{PoolID: a.PoolID, Fallback: a.Fallback,
		LocalizedSenderPreferred: effective(a.LocalizedSenderPreferred, false), PredictableSenderPreferred: effective(a.PredictableSenderPreferred, false)})
	if err != nil {
		return infer.CreateResponse[NumberPoolState]{}, fmt.Errorf("create number pool %q: %w", a.PoolID, err)
	}
	st := NumberPoolState{NumberPoolArgs: a, AccountID: c.apiKey}
	if w.AccountID != "" {
		st.AccountID = w.AccountID
	}
	return infer.CreateResponse[NumberPoolState]{ID: a.PoolID, Output: st}, nil
}

func (r *NumberPool) Read(ctx context.Context, req infer.ReadRequest[NumberPoolArgs, NumberPoolState]) (infer.ReadResponse[NumberPoolArgs, NumberPoolState], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.ReadResponse[NumberPoolArgs, NumberPoolState]{}, err
	}
	w, err := c.GetPool(ctx, req.ID)
	if err != nil {
		return infer.ReadResponse[NumberPoolArgs, NumberPoolState]{}, fmt.Errorf("read number pool %s: %w", req.ID, err)
	}
	if w == nil {
		if err := notFoundOnImport("number pool", req.ID, req.Inputs, req.State); err != nil {
			return infer.ReadResponse[NumberPoolArgs, NumberPoolState]{}, err
		}
		return infer.ReadResponse[NumberPoolArgs, NumberPoolState]{}, nil
	}
	if w.PoolID == "" {
		w.PoolID = req.ID
	}
	st := poolStateFrom(w, c.apiKey, req.Inputs)
	return infer.ReadResponse[NumberPoolArgs, NumberPoolState]{ID: req.ID, Inputs: st.NumberPoolArgs, State: st}, nil
}

func (r *NumberPool) Diff(ctx context.Context, req infer.DiffRequest[NumberPoolArgs, NumberPoolState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.NumberPoolArgs
	if in.PoolID != old.PoolID {
		d["poolId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.Fallback != old.Fallback {
		d["fallback"] = p.PropertyDiff{Kind: p.Update}
	}
	if effective(in.LocalizedSenderPreferred, false) != effective(old.LocalizedSenderPreferred, false) {
		d["localizedSenderPreferred"] = p.PropertyDiff{Kind: p.Update}
	}
	if effective(in.PredictableSenderPreferred, false) != effective(old.PredictableSenderPreferred, false) {
		d["predictableSenderPreferred"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *NumberPool) Update(ctx context.Context, req infer.UpdateRequest[NumberPoolArgs, NumberPoolState]) (infer.UpdateResponse[NumberPoolState], error) {
	a := req.Inputs
	if req.DryRun {
		st := req.State
		st.NumberPoolArgs = a
		return infer.UpdateResponse[NumberPoolState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[NumberPoolState]{}, err
	}
	// Unset means false, so send the effective values: removing a flag from the program must clear it.
	loc, pred := effective(a.LocalizedSenderPreferred, false), effective(a.PredictableSenderPreferred, false)
	if _, err := c.PatchPool(ctx, req.ID, wirePoolPatch{Fallback: &a.Fallback,
		LocalizedSenderPreferred: &loc, PredictableSenderPreferred: &pred}); err != nil {
		return infer.UpdateResponse[NumberPoolState]{}, fmt.Errorf("update number pool %s: %w", req.ID, err)
	}
	st := req.State
	st.NumberPoolArgs = a
	return infer.UpdateResponse[NumberPoolState]{Output: st}, nil
}

func (r *NumberPool) Delete(ctx context.Context, req infer.DeleteRequest[NumberPoolState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.DeletePool(ctx, req.ID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete number pool %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// NumberPoolNumber
// ---------------------------------------------------------------------------

// NumberPoolNumber places one owned number in a NumberPool.
type NumberPoolNumber struct{}

// NumberPoolNumberArgs are the inputs of a NumberPoolNumber.
type NumberPoolNumberArgs struct {
	PoolID string `pulumi:"poolId"`
	Number string `pulumi:"number"`
}

func (a *NumberPoolNumberArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.PoolID, "ID of the pool to add the number to. Changing it replaces the membership.")
	an.Describe(&a.Number, "The owned number in E.164 format without the leading plus (7-15 digits). Changing it replaces the membership.")
}

// NumberPoolNumberState is the persisted state of a NumberPoolNumber.
type NumberPoolNumberState struct {
	NumberPoolNumberArgs
}

func (r *NumberPoolNumber) Annotate(a infer.Annotator) {
	a.Describe(&r, "Membership of one owned number in a Vonage number pool. The ID is poolId/number. Removing the resource removes the number from the pool and never releases the number itself.")
}

var _ infer.CustomResource[NumberPoolNumberArgs, NumberPoolNumberState] = (*NumberPoolNumber)(nil)
var _ infer.CustomCheck[NumberPoolNumberArgs] = (*NumberPoolNumber)(nil)
var _ infer.CustomRead[NumberPoolNumberArgs, NumberPoolNumberState] = (*NumberPoolNumber)(nil)
var _ infer.CustomDiff[NumberPoolNumberArgs, NumberPoolNumberState] = (*NumberPoolNumber)(nil)
var _ infer.CustomDelete[NumberPoolNumberState] = (*NumberPoolNumber)(nil)

func (r *NumberPoolNumber) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[NumberPoolNumberArgs], error) {
	args, failures, err := infer.DefaultCheck[NumberPoolNumberArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[NumberPoolNumberArgs]{Inputs: args, Failures: failures}, err
	}
	if strings.TrimSpace(args.PoolID) == "" || strings.Contains(args.PoolID, "/") {
		failures = append(failures, p.CheckFailure{Property: "poolId", Reason: "poolId must be non-empty and must not contain \"/\""})
	}
	if !poolNumberRE.MatchString(args.Number) {
		failures = append(failures, p.CheckFailure{Property: "number", Reason: "number must be 7-15 digits with no leading plus"})
	}
	return infer.CheckResponse[NumberPoolNumberArgs]{Inputs: args, Failures: failures}, nil
}

func poolNumberID(poolID, number string) string { return poolID + "/" + number }

func (r *NumberPoolNumber) Create(ctx context.Context, req infer.CreateRequest[NumberPoolNumberArgs]) (infer.CreateResponse[NumberPoolNumberState], error) {
	a := req.Inputs
	st := NumberPoolNumberState{NumberPoolNumberArgs: a}
	if req.DryRun {
		return infer.CreateResponse[NumberPoolNumberState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[NumberPoolNumberState]{}, err
	}
	if err := c.AddPoolNumber(ctx, a.PoolID, a.Number); err != nil {
		return infer.CreateResponse[NumberPoolNumberState]{}, fmt.Errorf("add number %s to pool %q: %w", a.Number, a.PoolID, err)
	}
	// A 207 partial success is easy to miss, so confirm the membership before recording it.
	nums, found, err := c.ListPoolNumbers(ctx, a.PoolID)
	id := poolNumberID(a.PoolID, a.Number)
	if err != nil {
		return infer.CreateResponse[NumberPoolNumberState]{ID: id, Output: st},
			infer.ResourceInitFailedError{Reasons: []string{fmt.Sprintf("number added but reading the pool back failed: %v", err)}}
	}
	if !found || !containsString(nums, a.Number) {
		// The membership may exist (listing lag, or a partial-success shape the spec does not pin
		// down), so keep the ID: dropping it would make the next apply add the number again.
		return infer.CreateResponse[NumberPoolNumberState]{ID: id, Output: st},
			infer.ResourceInitFailedError{Reasons: []string{fmt.Sprintf("number %s was accepted but is not yet listed in pool %q", a.Number, a.PoolID)}}
	}
	return infer.CreateResponse[NumberPoolNumberState]{ID: id, Output: st}, nil
}

func containsString(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}

func (r *NumberPoolNumber) Read(ctx context.Context, req infer.ReadRequest[NumberPoolNumberArgs, NumberPoolNumberState]) (infer.ReadResponse[NumberPoolNumberArgs, NumberPoolNumberState], error) {
	poolID, number, ok := strings.Cut(req.ID, "/")
	if !ok || poolID == "" || !poolNumberRE.MatchString(number) {
		return infer.ReadResponse[NumberPoolNumberArgs, NumberPoolNumberState]{},
			fmt.Errorf("invalid NumberPoolNumber ID %q: expected poolId/number", req.ID)
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.ReadResponse[NumberPoolNumberArgs, NumberPoolNumberState]{}, err
	}
	nums, found, err := c.ListPoolNumbers(ctx, poolID)
	if err != nil {
		return infer.ReadResponse[NumberPoolNumberArgs, NumberPoolNumberState]{}, fmt.Errorf("read pool numbers %s: %w", req.ID, err)
	}
	if !found || !containsString(nums, number) {
		if err := notFoundOnImport("number pool number", req.ID, req.Inputs, req.State); err != nil {
			return infer.ReadResponse[NumberPoolNumberArgs, NumberPoolNumberState]{}, err
		}
		return infer.ReadResponse[NumberPoolNumberArgs, NumberPoolNumberState]{}, nil
	}
	args := NumberPoolNumberArgs{PoolID: poolID, Number: number}
	return infer.ReadResponse[NumberPoolNumberArgs, NumberPoolNumberState]{ID: req.ID, Inputs: args, State: NumberPoolNumberState{NumberPoolNumberArgs: args}}, nil
}

func (r *NumberPoolNumber) Diff(ctx context.Context, req infer.DiffRequest[NumberPoolNumberArgs, NumberPoolNumberState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	if req.Inputs.PoolID != req.State.PoolID {
		d["poolId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if req.Inputs.Number != req.State.Number {
		d["number"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DeleteBeforeReplace: true, DetailedDiff: d}, nil
}

func (r *NumberPoolNumber) Delete(ctx context.Context, req infer.DeleteRequest[NumberPoolNumberState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.RemovePoolNumber(ctx, req.State.PoolID, req.State.Number); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("remove number %s from pool %q: %w", req.State.Number, req.State.PoolID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// Lookups
// ---------------------------------------------------------------------------

// GetNumberPool looks up one number pool.
type GetNumberPool struct{}

// GetNumberPoolArgs are the lookup arguments.
type GetNumberPoolArgs struct {
	PoolID string `pulumi:"poolId"`
}

func (a *GetNumberPoolArgs) Annotate(an infer.Annotator) { an.Describe(&a.PoolID, "ID of the pool.") }

// GetNumberPoolResult is a pool's configuration.
type GetNumberPoolResult struct {
	PoolID                     string `pulumi:"poolId"`
	AccountID                  string `pulumi:"accountId"`
	Fallback                   string `pulumi:"fallback"`
	LocalizedSenderPreferred   bool   `pulumi:"localizedSenderPreferred"`
	PredictableSenderPreferred bool   `pulumi:"predictableSenderPreferred"`
}

func (a *GetNumberPoolResult) Annotate(an infer.Annotator) {
	an.Describe(&a.PoolID, "Pool ID.")
	an.Describe(&a.AccountID, "Owning account ID (the API key).")
	an.Describe(&a.Fallback, "Fallback behaviour: Pool, CustomFrom or Reject.")
	an.Describe(&a.LocalizedSenderPreferred, "Whether a number local to the destination is preferred.")
	an.Describe(&a.PredictableSenderPreferred, "Whether the same number is reused for the same recipient.")
}

func (f *GetNumberPool) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a number pool by ID. Fails if it does not exist.")
}

func poolResult(w wirePool, acct string) GetNumberPoolResult {
	if w.AccountID != "" {
		acct = w.AccountID
	}
	return GetNumberPoolResult{PoolID: w.PoolID, AccountID: acct, Fallback: w.Fallback,
		LocalizedSenderPreferred: w.LocalizedSenderPreferred, PredictableSenderPreferred: w.PredictableSenderPreferred}
}

func (f *GetNumberPool) Invoke(ctx context.Context, req infer.FunctionRequest[GetNumberPoolArgs]) (infer.FunctionResponse[GetNumberPoolResult], error) {
	id := req.Input.PoolID
	if id == "" {
		return infer.FunctionResponse[GetNumberPoolResult]{}, fmt.Errorf("getNumberPool: poolId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNumberPoolResult]{}, err
	}
	w, err := c.GetPool(ctx, id)
	if err != nil {
		return infer.FunctionResponse[GetNumberPoolResult]{}, fmt.Errorf("getNumberPool: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[GetNumberPoolResult]{}, fmt.Errorf("getNumberPool: pool %q was not found", id)
	}
	if w.PoolID == "" {
		w.PoolID = id
	}
	return infer.FunctionResponse[GetNumberPoolResult]{Output: poolResult(*w, c.apiKey)}, nil
}

// GetNumberPools lists every number pool of the account.
type GetNumberPools struct{}

// GetNumberPoolsArgs has no arguments.
type GetNumberPoolsArgs struct{}

// GetNumberPoolsResult lists the pools.
type GetNumberPoolsResult struct {
	Pools []GetNumberPoolResult `pulumi:"pools"`
}

func (a *GetNumberPoolsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Pools, "Every number pool of the account.")
}

func (f *GetNumberPools) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the account's number pools, following every page.")
}

func (f *GetNumberPools) Invoke(ctx context.Context, req infer.FunctionRequest[GetNumberPoolsArgs]) (infer.FunctionResponse[GetNumberPoolsResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNumberPoolsResult]{}, err
	}
	ws, err := c.ListPools(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNumberPoolsResult]{}, fmt.Errorf("getNumberPools: %w", err)
	}
	out := GetNumberPoolsResult{Pools: []GetNumberPoolResult{}}
	for _, w := range ws {
		out.Pools = append(out.Pools, poolResult(w, c.apiKey))
	}
	return infer.FunctionResponse[GetNumberPoolsResult]{Output: out}, nil
}

// GetNumberPoolNumbers lists the numbers in a pool.
type GetNumberPoolNumbers struct{}

// GetNumberPoolNumbersArgs are the lookup arguments.
type GetNumberPoolNumbersArgs struct {
	PoolID string `pulumi:"poolId"`
}

func (a *GetNumberPoolNumbersArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.PoolID, "ID of the pool.")
}

// GetNumberPoolNumbersResult lists the pool's numbers.
type GetNumberPoolNumbersResult struct {
	Numbers []string `pulumi:"numbers"`
}

func (a *GetNumberPoolNumbersResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Numbers, "Numbers in the pool, in E.164 format without the leading plus.")
}

func (f *GetNumberPoolNumbers) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the numbers in a pool, following every page. Fails if the pool does not exist.")
}

func (f *GetNumberPoolNumbers) Invoke(ctx context.Context, req infer.FunctionRequest[GetNumberPoolNumbersArgs]) (infer.FunctionResponse[GetNumberPoolNumbersResult], error) {
	id := req.Input.PoolID
	if id == "" {
		return infer.FunctionResponse[GetNumberPoolNumbersResult]{}, fmt.Errorf("getNumberPoolNumbers: poolId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNumberPoolNumbersResult]{}, err
	}
	nums, found, err := c.ListPoolNumbers(ctx, id)
	if err != nil {
		return infer.FunctionResponse[GetNumberPoolNumbersResult]{}, fmt.Errorf("getNumberPoolNumbers: %w", err)
	}
	if !found {
		return infer.FunctionResponse[GetNumberPoolNumbersResult]{}, fmt.Errorf("getNumberPoolNumbers: pool %q was not found", id)
	}
	return infer.FunctionResponse[GetNumberPoolNumbersResult]{Output: GetNumberPoolNumbersResult{Numbers: nums}}, nil
}

// GetNumberPoolNumber looks up one number's membership of a pool.
type GetNumberPoolNumber struct{}

// GetNumberPoolNumberArgs are the lookup arguments.
type GetNumberPoolNumberArgs struct {
	PoolID string `pulumi:"poolId"`
	Number string `pulumi:"number"`
}

func (a *GetNumberPoolNumberArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.PoolID, "ID of the pool.")
	an.Describe(&a.Number, "The number to look for, 7-15 digits with no leading plus.")
}

// GetNumberPoolNumberResult confirms a membership.
type GetNumberPoolNumberResult struct {
	PoolID string `pulumi:"poolId"`
	Number string `pulumi:"number"`
}

func (a *GetNumberPoolNumberResult) Annotate(an infer.Annotator) {
	an.Describe(&a.PoolID, "ID of the pool.")
	an.Describe(&a.Number, "The number, which is in the pool.")
}

func (f *GetNumberPoolNumber) Annotate(a infer.Annotator) {
	a.Describe(&f, "Check that a number is in a pool. Fails if the pool does not exist or the number is not in it.")
}

func (f *GetNumberPoolNumber) Invoke(ctx context.Context, req infer.FunctionRequest[GetNumberPoolNumberArgs]) (infer.FunctionResponse[GetNumberPoolNumberResult], error) {
	in := req.Input
	if in.PoolID == "" {
		return infer.FunctionResponse[GetNumberPoolNumberResult]{}, fmt.Errorf("getNumberPoolNumber: poolId must not be empty")
	}
	if !poolNumberRE.MatchString(in.Number) {
		return infer.FunctionResponse[GetNumberPoolNumberResult]{}, fmt.Errorf("getNumberPoolNumber: number must be 7-15 digits with no leading plus")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetNumberPoolNumberResult]{}, err
	}
	nums, found, err := c.ListPoolNumbers(ctx, in.PoolID)
	if err != nil {
		return infer.FunctionResponse[GetNumberPoolNumberResult]{}, fmt.Errorf("getNumberPoolNumber: %w", err)
	}
	if !found {
		return infer.FunctionResponse[GetNumberPoolNumberResult]{}, fmt.Errorf("getNumberPoolNumber: pool %q was not found", in.PoolID)
	}
	if !containsString(nums, in.Number) {
		return infer.FunctionResponse[GetNumberPoolNumberResult]{}, fmt.Errorf("getNumberPoolNumber: number %s is not in pool %q", in.Number, in.PoolID)
	}
	return infer.FunctionResponse[GetNumberPoolNumberResult]{Output: GetNumberPoolNumberResult{PoolID: in.PoolID, Number: in.Number}}, nil
}
