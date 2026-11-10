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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// Users API (https://developer.vonage.com/en/api/conversation, the "users" part of the Conversation
// API). It authenticates with an application JWT, not the account key and secret, and every user
// belongs to the application named in the JWT. The request and response shapes below follow the
// published API as implemented by the vonage-python-sdk and vonage-node-sdk and have not been
// exercised against a live Vonage account.

const (
	userPageSize = 100
	// maxUserPages bounds the pages a user listing follows.
	maxUserPages = 100
)

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

// flexString decodes a JSON string or number into a string (the API documents numbers as
// strings for the messaging channels but is loose about it).
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = flexString(v)
		return nil
	}
	*f = flexString(s)
	return nil
}

type wirePstn struct {
	Number int64 `json:"number"`
}

type wireSip struct {
	URI      string `json:"uri"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type wireVbc struct {
	Extension flexString `json:"extension"`
}

type wireWebsocket struct {
	URI         string            `json:"uri"`
	ContentType string            `json:"content-type,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type wireNumberChannel struct {
	Number flexString `json:"number"`
}

type wireMessenger struct {
	ID flexString `json:"id"`
}

type wireChannels struct {
	Pstn      []wirePstn          `json:"pstn,omitempty"`
	Sip       []wireSip           `json:"sip,omitempty"`
	Vbc       []wireVbc           `json:"vbc,omitempty"`
	Websocket []wireWebsocket     `json:"websocket,omitempty"`
	Sms       []wireNumberChannel `json:"sms,omitempty"`
	Mms       []wireNumberChannel `json:"mms,omitempty"`
	Whatsapp  []wireNumberChannel `json:"whatsapp,omitempty"`
	Viber     []wireNumberChannel `json:"viber,omitempty"`
	Messenger []wireMessenger     `json:"messenger,omitempty"`
}

type wireUserProps struct {
	CustomData map[string]any `json:"custom_data,omitempty"`
}

type wireUser struct {
	ID          string         `json:"id,omitempty"`
	Name        string         `json:"name,omitempty"`
	DisplayName string         `json:"display_name,omitempty"`
	ImageURL    string         `json:"image_url,omitempty"`
	Properties  *wireUserProps `json:"properties,omitempty"`
	Channels    *wireChannels  `json:"channels,omitempty"`
}

// wireUserPage is one page of the HAL user listing.
type wireUserPage struct {
	Embedded struct {
		Users []wireUser `json:"users"`
	} `json:"_embedded"`
	Links struct {
		Next *struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links"`
}

// userChannelKeys lists the channel types in a stable order.
var userChannelKeys = []string{"pstn", "sip", "vbc", "websocket", "sms", "mms", "whatsapp", "viber", "messenger"}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

func userPath(id string) string { return "/v1/users/" + url.PathEscape(id) }

// CreateUser creates a user and returns it as stored.
func (c *Client) CreateUser(ctx context.Context, in wireUser) (*wireUser, error) {
	var out wireUser
	if err := c.userJSON(ctx, http.MethodPost, "/v1/users", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUser returns nil, nil when the user does not exist.
func (c *Client) GetUser(ctx context.Context, id string) (*wireUser, error) {
	var out wireUser
	err := c.userJSON(ctx, http.MethodGet, userPath(id), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if out.ID == "" {
		out.ID = id
	}
	return &out, nil
}

// PatchUser updates a user with a partial body.
func (c *Client) PatchUser(ctx context.Context, id string, body map[string]any) error {
	return c.userJSON(ctx, http.MethodPatch, userPath(id), nil, body, nil)
}

// DeleteUser deletes a user; a missing user is not an error.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	err := c.userJSON(ctx, http.MethodDelete, userPath(id), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// ListUsers returns the summaries of every user of the application, optionally restricted to
// one name, following the cursor links. It fails loudly if the cursor does not advance or the
// page cap is hit, rather than returning a silently truncated list.
func (c *Client) ListUsers(ctx context.Context, name string) ([]wireUser, error) {
	out := []wireUser{}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < maxUserPages; page++ {
		q := url.Values{"page_size": {strconv.Itoa(userPageSize)}, "order": {"asc"}}
		if name != "" {
			q.Set("name", name)
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var pg wireUserPage
		if err := c.userJSON(ctx, http.MethodGet, "/v1/users", q, nil, &pg); err != nil {
			return nil, err
		}
		out = append(out, pg.Embedded.Users...)
		if pg.Links.Next == nil || pg.Links.Next.Href == "" {
			return out, nil
		}
		u, err := url.Parse(pg.Links.Next.Href)
		if err != nil {
			return nil, fmt.Errorf("vonage: GET /v1/users: unparseable next link %q: %w", pg.Links.Next.Href, err)
		}
		cursor = u.Query().Get("cursor")
		if cursor == "" || seen[cursor] {
			return nil, fmt.Errorf("vonage: GET /v1/users: next link %q does not advance the cursor, refusing to guess", pg.Links.Next.Href)
		}
		seen[cursor] = true
	}
	return nil, fmt.Errorf("vonage: GET /v1/users: more than %d pages", maxUserPages)
}

// ---------------------------------------------------------------------------
// Resource model
// ---------------------------------------------------------------------------

// UserPstnChannel is a phone number a user can be called on.
type UserPstnChannel struct {
	Number int `pulumi:"number"`
}

func (a *UserPstnChannel) Annotate(an infer.Annotator) {
	an.Describe(&a.Number, "The phone number in E.164 format without the leading plus, as an integer (for example 14155550100).")
}

// UserSipChannel is a SIP endpoint of a user.
type UserSipChannel struct {
	URI      string  `pulumi:"uri"`
	Username *string `pulumi:"username,optional"`
	Password *string `pulumi:"password,optional" provider:"secret"`
}

func (a *UserSipChannel) Annotate(an infer.Annotator) {
	an.Describe(&a.URI, "The SIP URI, starting with sip: or sips:.")
	an.Describe(&a.Username, "Username for SIP authentication.")
	an.Describe(&a.Password, "Password for SIP authentication. The API never returns it, so the value recorded in state is the one last set by this program; a changed password is applied but a password changed outside Pulumi is not detected.")
}

// UserVbcChannel is a Vonage Business Cloud extension of a user.
type UserVbcChannel struct {
	Extension string `pulumi:"extension"`
}

func (a *UserVbcChannel) Annotate(an infer.Annotator) {
	an.Describe(&a.Extension, "The Vonage Business Cloud extension.")
}

// UserWebsocketChannel is a WebSocket endpoint of a user.
type UserWebsocketChannel struct {
	URI         string            `pulumi:"uri"`
	ContentType *string           `pulumi:"contentType,optional"`
	Headers     map[string]string `pulumi:"headers,optional"`
}

func (a *UserWebsocketChannel) Annotate(an infer.Annotator) {
	an.Describe(&a.URI, "The WebSocket URI, starting with ws:// or wss://.")
	an.Describe(&a.ContentType, "The audio format, for example audio/l16;rate=16000 (sent to Vonage as content-type).")
	an.Describe(&a.Headers, "Extra HTTP headers sent when Vonage connects to the WebSocket.")
}

// UserNumberChannel is a messaging endpoint identified by a number (SMS, MMS, WhatsApp, Viber).
type UserNumberChannel struct {
	Number string `pulumi:"number"`
}

func (a *UserNumberChannel) Annotate(an infer.Annotator) {
	an.Describe(&a.Number, "The number in E.164 format without the leading plus.")
}

// UserMessengerChannel is a Facebook Messenger endpoint of a user.
type UserMessengerChannel struct {
	ID string `pulumi:"id"`
}

func (a *UserMessengerChannel) Annotate(an infer.Annotator) {
	an.Describe(&a.ID, "The Facebook Messenger user ID.")
}

// UserChannels are the communication channels a user can be reached on.
type UserChannels struct {
	Pstn      []UserPstnChannel      `pulumi:"pstn,optional"`
	Sip       []UserSipChannel       `pulumi:"sip,optional"`
	Vbc       []UserVbcChannel       `pulumi:"vbc,optional"`
	Websocket []UserWebsocketChannel `pulumi:"websocket,optional"`
	Sms       []UserNumberChannel    `pulumi:"sms,optional"`
	Mms       []UserNumberChannel    `pulumi:"mms,optional"`
	Whatsapp  []UserNumberChannel    `pulumi:"whatsapp,optional"`
	Viber     []UserNumberChannel    `pulumi:"viber,optional"`
	Messenger []UserMessengerChannel `pulumi:"messenger,optional"`
}

func (a *UserChannels) Annotate(an infer.Annotator) {
	an.Describe(&a.Pstn, "Phone numbers (PSTN) the user can be called on.")
	an.Describe(&a.Sip, "SIP endpoints of the user.")
	an.Describe(&a.Vbc, "Vonage Business Cloud extensions of the user.")
	an.Describe(&a.Websocket, "WebSocket endpoints of the user.")
	an.Describe(&a.Sms, "Numbers the user can receive SMS on.")
	an.Describe(&a.Mms, "Numbers the user can receive MMS on.")
	an.Describe(&a.Whatsapp, "WhatsApp numbers of the user.")
	an.Describe(&a.Viber, "Viber numbers of the user.")
	an.Describe(&a.Messenger, "Facebook Messenger IDs of the user.")
}

// User manages a Vonage Users API user: a person or endpoint that belongs to one Vonage
// application and can be a member of conversations.
type User struct{}

// UserArgs are the inputs of a User.
type UserArgs struct {
	Name        *string        `pulumi:"name,optional"`
	DisplayName *string        `pulumi:"displayName,optional"`
	ImageURL    *string        `pulumi:"imageUrl,optional"`
	CustomData  map[string]any `pulumi:"customData,optional"`
	Channels    *UserChannels  `pulumi:"channels,optional"`
}

func (a *UserArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "Unique name of the user within the application. When omitted Vonage generates one (NAM-<uuid>), "+
		"which is recorded in state but is not a difference. Changing an explicit name replaces the user, because the API's update "+
		"call is not documented to rename.")
	an.Describe(&a.DisplayName, "Human-friendly name shown in clients.")
	an.Describe(&a.ImageURL, "URL of an image for the user.")
	an.Describe(&a.CustomData, "Free-form JSON object stored on the user (properties.custom_data).")
	an.Describe(&a.Channels, "The channels the user can be reached on. Each list is replaced as a whole on update.")
}

// UserState is the persisted state of a User.
type UserState struct {
	UserArgs
}

func (r *User) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Vonage Users API user (USR-<uuid>), scoped to the application whose applicationId and privateKey the provider is configured with. "+
		"Authenticates with an application JWT signed from the private key, so it needs only applicationId and privateKey, not apiKey and apiSecret. "+
		"The resource ID is the user ID. Shapes follow the published Users API and were not verified against a live account.")
}

var _ infer.CustomResource[UserArgs, UserState] = (*User)(nil)
var _ infer.CustomCheck[UserArgs] = (*User)(nil)
var _ infer.CustomRead[UserArgs, UserState] = (*User)(nil)
var _ infer.CustomUpdate[UserArgs, UserState] = (*User)(nil)
var _ infer.CustomDiff[UserArgs, UserState] = (*User)(nil)
var _ infer.CustomDelete[UserState] = (*User)(nil)

// ---------------------------------------------------------------------------
// Conversions
// ---------------------------------------------------------------------------

func channelsToWire(c *UserChannels) wireChannels {
	var w wireChannels
	if c == nil {
		return w
	}
	for _, x := range c.Pstn {
		w.Pstn = append(w.Pstn, wirePstn{Number: int64(x.Number)})
	}
	for _, x := range c.Sip {
		w.Sip = append(w.Sip, wireSip{URI: x.URI, Username: deref(x.Username), Password: deref(x.Password)})
	}
	for _, x := range c.Vbc {
		w.Vbc = append(w.Vbc, wireVbc{Extension: flexString(x.Extension)})
	}
	for _, x := range c.Websocket {
		w.Websocket = append(w.Websocket, wireWebsocket{URI: x.URI, ContentType: deref(x.ContentType), Headers: x.Headers})
	}
	num := func(in []UserNumberChannel) (out []wireNumberChannel) {
		for _, x := range in {
			out = append(out, wireNumberChannel{Number: flexString(x.Number)})
		}
		return out
	}
	w.Sms, w.Mms, w.Whatsapp, w.Viber = num(c.Sms), num(c.Mms), num(c.Whatsapp), num(c.Viber)
	for _, x := range c.Messenger {
		w.Messenger = append(w.Messenger, wireMessenger{ID: flexString(x.ID)})
	}
	return w
}

// channelsFromWire converts the server's channels back to inputs. The API never returns SIP
// passwords, so each one is taken from prior (matched on URI and username).
func channelsFromWire(w *wireChannels, prior *UserChannels) *UserChannels {
	if w == nil {
		return nil
	}
	var c UserChannels
	for _, x := range w.Pstn {
		c.Pstn = append(c.Pstn, UserPstnChannel{Number: int(x.Number)})
	}
	for _, x := range w.Sip {
		s := UserSipChannel{URI: x.URI, Username: nonEmptyPtr(x.Username), Password: nonEmptyPtr(x.Password)}
		if s.Password == nil && prior != nil {
			for _, pr := range prior.Sip {
				if pr.URI == x.URI && deref(pr.Username) == x.Username {
					s.Password = pr.Password
				}
			}
		}
		c.Sip = append(c.Sip, s)
	}
	for _, x := range w.Vbc {
		c.Vbc = append(c.Vbc, UserVbcChannel{Extension: string(x.Extension)})
	}
	for _, x := range w.Websocket {
		c.Websocket = append(c.Websocket, UserWebsocketChannel{URI: x.URI, ContentType: nonEmptyPtr(x.ContentType), Headers: x.Headers})
	}
	num := func(in []wireNumberChannel) (out []UserNumberChannel) {
		for _, x := range in {
			out = append(out, UserNumberChannel{Number: string(x.Number)})
		}
		return out
	}
	c.Sms, c.Mms, c.Whatsapp, c.Viber = num(w.Sms), num(w.Mms), num(w.Whatsapp), num(w.Viber)
	for _, x := range w.Messenger {
		c.Messenger = append(c.Messenger, UserMessengerChannel{ID: string(x.ID)})
	}
	if reflect.DeepEqual(c, UserChannels{}) {
		return nil
	}
	return &c
}

// canonJSON renders v with sorted map keys so equal values compare equal.
func canonJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func canonChannels(c *UserChannels) string { return canonJSON(channelsToWire(c)) }

func canonCustomData(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	return canonJSON(m)
}

// createBody is the POST body for the desired inputs.
func createBody(a UserArgs) wireUser {
	w := wireUser{Name: deref(a.Name), DisplayName: deref(a.DisplayName), ImageURL: deref(a.ImageURL)}
	if len(a.CustomData) > 0 {
		w.Properties = &wireUserProps{CustomData: a.CustomData}
	}
	if ch := channelsToWire(a.Channels); canonJSON(ch) != "{}" {
		w.Channels = &ch
	}
	return w
}

// patchBody is the PATCH body that brings a user holding prev to the desired inputs. A value
// that was set and is now gone is cleared explicitly (null for scalars, {} for custom data,
// an empty list for a channel type); the clearing forms are unverified against a live account.
func patchBody(a, prev UserArgs) map[string]any {
	body := map[string]any{}
	switch {
	case a.DisplayName != nil && *a.DisplayName != "":
		body["display_name"] = *a.DisplayName
	case deref(prev.DisplayName) != "":
		body["display_name"] = nil
	}
	switch {
	case a.ImageURL != nil && *a.ImageURL != "":
		body["image_url"] = *a.ImageURL
	case deref(prev.ImageURL) != "":
		body["image_url"] = nil
	}
	switch {
	case len(a.CustomData) > 0:
		body["properties"] = map[string]any{"custom_data": a.CustomData}
	case len(prev.CustomData) > 0:
		body["properties"] = map[string]any{"custom_data": map[string]any{}}
	}
	want := toRawMap(channelsToWire(a.Channels))
	had := toRawMap(channelsToWire(prev.Channels))
	ch := map[string]any{}
	for _, k := range userChannelKeys {
		switch {
		case want[k] != nil:
			ch[k] = want[k]
		case had[k] != nil:
			ch[k] = []any{}
		}
	}
	if len(ch) > 0 {
		body["channels"] = ch
	}
	return body
}

func toRawMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

// userArgsFromWire converts a server user to inputs. The name is surfaced only when the prior
// inputs carried one or there are no prior inputs at all (an import), so a generated name never
// becomes a diff against a program that leaves it unset.
func userArgsFromWire(w *wireUser, prior UserArgs) UserArgs {
	a := UserArgs{DisplayName: nonEmptyPtr(w.DisplayName), ImageURL: nonEmptyPtr(w.ImageURL),
		Channels: channelsFromWire(w.Channels, prior.Channels)}
	if w.Properties != nil && len(w.Properties.CustomData) > 0 {
		a.CustomData = w.Properties.CustomData
	}
	if prior.Name != nil || reflect.ValueOf(prior).IsZero() {
		a.Name = nonEmptyPtr(w.Name)
	}
	return a
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (r *User) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[UserArgs], error) {
	args, failures, err := infer.DefaultCheck[UserArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[UserArgs]{Inputs: args, Failures: failures}, err
	}
	fail := func(prop, reason string) { failures = append(failures, p.CheckFailure{Property: prop, Reason: reason}) }
	if args.Name != nil && strings.TrimSpace(*args.Name) == "" {
		fail("name", "name must not be empty when set (leave it unset to let Vonage generate one)")
	}
	if ch := args.Channels; ch != nil {
		for i, x := range ch.Pstn {
			if x.Number <= 0 {
				fail(fmt.Sprintf("channels.pstn[%d].number", i), "number must be a positive integer in E.164 format without the plus")
			}
		}
		for i, x := range ch.Sip {
			if !strings.HasPrefix(x.URI, "sip:") && !strings.HasPrefix(x.URI, "sips:") {
				fail(fmt.Sprintf("channels.sip[%d].uri", i), "uri must start with sip: or sips:")
			}
		}
		for i, x := range ch.Websocket {
			if !strings.HasPrefix(x.URI, "ws://") && !strings.HasPrefix(x.URI, "wss://") {
				fail(fmt.Sprintf("channels.websocket[%d].uri", i), "uri must start with ws:// or wss://")
			}
		}
	}
	return infer.CheckResponse[UserArgs]{Inputs: args, Failures: failures}, nil
}

func (r *User) Create(ctx context.Context, req infer.CreateRequest[UserArgs]) (infer.CreateResponse[UserState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[UserState]{Output: UserState{UserArgs: a}}, nil
	}
	c, err := jwtClientFor(ctx)
	if err != nil {
		return infer.CreateResponse[UserState]{}, err
	}
	w, err := c.CreateUser(ctx, createBody(a))
	if err != nil {
		return infer.CreateResponse[UserState]{}, fmt.Errorf("create user: %w", err)
	}
	if w.ID == "" {
		return infer.CreateResponse[UserState]{}, fmt.Errorf("create user: the response carried no user id")
	}
	st := UserState{UserArgs: a}
	if w.Name != "" {
		st.Name = &w.Name
	}
	return infer.CreateResponse[UserState]{ID: w.ID, Output: st}, nil
}

func (r *User) Read(ctx context.Context, req infer.ReadRequest[UserArgs, UserState]) (infer.ReadResponse[UserArgs, UserState], error) {
	c, err := jwtClientFor(ctx)
	if err != nil {
		return infer.ReadResponse[UserArgs, UserState]{}, err
	}
	w, err := c.GetUser(ctx, req.ID)
	if err != nil {
		return infer.ReadResponse[UserArgs, UserState]{}, fmt.Errorf("read user %s: %w", req.ID, err)
	}
	if w == nil {
		if err := notFoundOnImport("user", req.ID, req.Inputs, req.State); err != nil {
			return infer.ReadResponse[UserArgs, UserState]{}, err
		}
		return infer.ReadResponse[UserArgs, UserState]{}, nil
	}
	in := userArgsFromWire(w, req.Inputs)
	st := UserState{UserArgs: userArgsFromWire(w, UserArgs{Channels: req.State.Channels})}
	st.Name = nonEmptyPtr(w.Name)
	return infer.ReadResponse[UserArgs, UserState]{ID: req.ID, Inputs: in, State: st}, nil
}

func (r *User) Diff(ctx context.Context, req infer.DiffRequest[UserArgs, UserState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.UserArgs
	if in.Name != nil && *in.Name != deref(old.Name) {
		d["name"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if deref(in.DisplayName) != deref(old.DisplayName) {
		d["displayName"] = p.PropertyDiff{Kind: p.Update}
	}
	if deref(in.ImageURL) != deref(old.ImageURL) {
		d["imageUrl"] = p.PropertyDiff{Kind: p.Update}
	}
	if canonCustomData(in.CustomData) != canonCustomData(old.CustomData) {
		d["customData"] = p.PropertyDiff{Kind: p.Update}
	}
	if canonChannels(in.Channels) != canonChannels(old.Channels) {
		d["channels"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *User) Update(ctx context.Context, req infer.UpdateRequest[UserArgs, UserState]) (infer.UpdateResponse[UserState], error) {
	a := req.Inputs
	st := UserState{UserArgs: a}
	st.Name = req.State.Name
	if a.Name != nil {
		st.Name = a.Name
	}
	if req.DryRun {
		return infer.UpdateResponse[UserState]{Output: st}, nil
	}
	c, err := jwtClientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[UserState]{}, err
	}
	if body := patchBody(a, req.State.UserArgs); len(body) > 0 {
		if err := c.PatchUser(ctx, req.ID, body); err != nil {
			return infer.UpdateResponse[UserState]{}, fmt.Errorf("update user %s: %w", req.ID, err)
		}
	}
	return infer.UpdateResponse[UserState]{Output: st}, nil
}

func (r *User) Delete(ctx context.Context, req infer.DeleteRequest[UserState]) (infer.DeleteResponse, error) {
	c, err := jwtClientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.DeleteUser(ctx, req.ID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete user %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// Lookups
// ---------------------------------------------------------------------------

// GetUser looks up one user by ID or by name.
type GetUser struct{}

// GetUserArgs are the lookup arguments; exactly one of userId and name is required.
type GetUserArgs struct {
	UserID *string `pulumi:"userId,optional"`
	Name   *string `pulumi:"name,optional"`
}

func (a *GetUserArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.UserID, "ID of the user (USR-<uuid>). Exactly one of userId and name is required.")
	an.Describe(&a.Name, "Exact name of the user. Exactly one of userId and name is required.")
}

// GetUserResult is a user's configuration. SIP passwords are never returned by the API.
type GetUserResult struct {
	UserID      string         `pulumi:"userId"`
	Name        string         `pulumi:"name"`
	DisplayName *string        `pulumi:"displayName,optional"`
	ImageURL    *string        `pulumi:"imageUrl,optional"`
	CustomData  map[string]any `pulumi:"customData,optional"`
	Channels    *UserChannels  `pulumi:"channels,optional"`
}

func (a *GetUserResult) Annotate(an infer.Annotator) {
	an.Describe(&a.UserID, "User ID (USR-<uuid>).")
	an.Describe(&a.Name, "Unique name of the user.")
	an.Describe(&a.DisplayName, "Human-friendly name, if set.")
	an.Describe(&a.ImageURL, "Image URL, if set.")
	an.Describe(&a.CustomData, "The user's custom data object, if any.")
	an.Describe(&a.Channels, "The user's channels, if any (SIP passwords are not returned by the API).")
}

func (f *GetUser) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a user of the configured application by ID or exact name. Needs applicationId and privateKey. Fails if the user does not exist.")
}

func (f *GetUser) Invoke(ctx context.Context, req infer.FunctionRequest[GetUserArgs]) (infer.FunctionResponse[GetUserResult], error) {
	id, name := deref(req.Input.UserID), deref(req.Input.Name)
	if (id == "") == (name == "") {
		return infer.FunctionResponse[GetUserResult]{}, fmt.Errorf("getUser: set exactly one of userId and name")
	}
	c, err := jwtClientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetUserResult]{}, err
	}
	if id == "" {
		list, err := c.ListUsers(ctx, name)
		if err != nil {
			return infer.FunctionResponse[GetUserResult]{}, fmt.Errorf("getUser: %w", err)
		}
		for _, u := range list {
			if u.Name == name {
				id = u.ID
				break
			}
		}
		if id == "" {
			return infer.FunctionResponse[GetUserResult]{}, fmt.Errorf("getUser: no user named %q was found", name)
		}
	}
	w, err := c.GetUser(ctx, id)
	if err != nil {
		return infer.FunctionResponse[GetUserResult]{}, fmt.Errorf("getUser: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[GetUserResult]{}, fmt.Errorf("getUser: user %q was not found", id)
	}
	a := userArgsFromWire(w, UserArgs{})
	return infer.FunctionResponse[GetUserResult]{Output: GetUserResult{UserID: w.ID, Name: w.Name,
		DisplayName: a.DisplayName, ImageURL: a.ImageURL, CustomData: a.CustomData, Channels: a.Channels}}, nil
}

// UserSummary is the short form of a user the list endpoint returns.
type UserSummary struct {
	UserID      string  `pulumi:"userId"`
	Name        string  `pulumi:"name"`
	DisplayName *string `pulumi:"displayName,optional"`
}

func (a *UserSummary) Annotate(an infer.Annotator) {
	an.Describe(&a.UserID, "User ID (USR-<uuid>).")
	an.Describe(&a.Name, "Unique name of the user.")
	an.Describe(&a.DisplayName, "Human-friendly name, if set.")
}

// GetUsers lists the users of the application.
type GetUsers struct{}

// GetUsersArgs are the lookup arguments.
type GetUsersArgs struct {
	Name *string `pulumi:"name,optional"`
}

func (a *GetUsersArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "Only return the user with exactly this name.")
}

// GetUsersResult lists the users.
type GetUsersResult struct {
	Users []UserSummary `pulumi:"users"`
}

func (a *GetUsersResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Users, "The users of the application. Use getUser for channels and custom data.")
}

func (f *GetUsers) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the users of the configured application, following every page. Needs applicationId and privateKey.")
}

func (f *GetUsers) Invoke(ctx context.Context, req infer.FunctionRequest[GetUsersArgs]) (infer.FunctionResponse[GetUsersResult], error) {
	c, err := jwtClientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetUsersResult]{}, err
	}
	name := deref(req.Input.Name)
	list, err := c.ListUsers(ctx, name)
	if err != nil {
		return infer.FunctionResponse[GetUsersResult]{}, fmt.Errorf("getUsers: %w", err)
	}
	out := GetUsersResult{Users: []UserSummary{}}
	for _, u := range list {
		if name != "" && u.Name != name {
			continue
		}
		out.Users = append(out.Users, UserSummary{UserID: u.ID, Name: u.Name, DisplayName: nonEmptyPtr(u.DisplayName)})
	}
	return infer.FunctionResponse[GetUsersResult]{Output: out}, nil
}
