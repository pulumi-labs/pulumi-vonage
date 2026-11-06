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
	"encoding/json"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// Application capabilities: video and network_apis
// ---------------------------------------------------------------------------

// VideoWebhook is a Video API webhook: a URL plus an active switch.
type VideoWebhook struct {
	Address string `pulumi:"address"`
	Active  *bool  `pulumi:"active,optional"`
}

func (w *VideoWebhook) Annotate(a infer.Annotator) {
	a.Describe(&w.Address, "The URL Vonage calls.")
	a.Describe(&w.Active, "Whether Vonage calls the webhook (server default true).")
}

// VideoWebhooks are the fifteen Video API webhooks an application can subscribe to.
type VideoWebhooks struct {
	ArchiveStatus       *VideoWebhook `pulumi:"archiveStatus,optional"`
	BroadcastStatus     *VideoWebhook `pulumi:"broadcastStatus,optional"`
	CaptionsStatus      *VideoWebhook `pulumi:"captionsStatus,optional"`
	ConnectionCreated   *VideoWebhook `pulumi:"connectionCreated,optional"`
	ConnectionDestroyed *VideoWebhook `pulumi:"connectionDestroyed,optional"`
	RenderStatus        *VideoWebhook `pulumi:"renderStatus,optional"`
	SessionCreated      *VideoWebhook `pulumi:"sessionCreated,optional"`
	SessionDestroyed    *VideoWebhook `pulumi:"sessionDestroyed,optional"`
	SessionNotification *VideoWebhook `pulumi:"sessionNotification,optional"`
	SipCallCreated      *VideoWebhook `pulumi:"sipCallCreated,optional"`
	SipCallDestroyed    *VideoWebhook `pulumi:"sipCallDestroyed,optional"`
	SipCallMuteForced   *VideoWebhook `pulumi:"sipCallMuteForced,optional"`
	SipCallUpdated      *VideoWebhook `pulumi:"sipCallUpdated,optional"`
	StreamCreated       *VideoWebhook `pulumi:"streamCreated,optional"`
	StreamDestroyed     *VideoWebhook `pulumi:"streamDestroyed,optional"`
}

func (w *VideoWebhooks) Annotate(a infer.Annotator) {
	a.Describe(&w.ArchiveStatus, "Receives archive (recording) status changes.")
	a.Describe(&w.BroadcastStatus, "Receives broadcast status changes.")
	a.Describe(&w.CaptionsStatus, "Receives live caption status changes.")
	a.Describe(&w.ConnectionCreated, "Receives an event when a client connects to a session.")
	a.Describe(&w.ConnectionDestroyed, "Receives an event when a client disconnects from a session.")
	a.Describe(&w.RenderStatus, "Receives Experience Composer render status changes.")
	a.Describe(&w.SessionCreated, "Receives an event when a session is created.")
	a.Describe(&w.SessionDestroyed, "Receives an event when a session is destroyed.")
	a.Describe(&w.SessionNotification, "Receives session notifications.")
	a.Describe(&w.SipCallCreated, "Receives an event when a SIP call is created.")
	a.Describe(&w.SipCallDestroyed, "Receives an event when a SIP call is destroyed.")
	a.Describe(&w.SipCallMuteForced, "Receives an event when a SIP call is force-muted.")
	a.Describe(&w.SipCallUpdated, "Receives an event when a SIP call is updated.")
	a.Describe(&w.StreamCreated, "Receives an event when a stream is created.")
	a.Describe(&w.StreamDestroyed, "Receives an event when a stream is destroyed.")
}

// VideoStorage configures recording storage and encryption for the video capability.
type VideoStorage struct {
	ServerSideEncryption *bool `pulumi:"serverSideEncryption,optional"`
	EndToEndEncryption   *bool `pulumi:"endToEndEncryption,optional"`
	CloudStorage         *bool `pulumi:"cloudStorage,optional"`
}

func (s *VideoStorage) Annotate(a infer.Annotator) {
	a.Describe(&s.ServerSideEncryption, "Whether recordings are encrypted server side.")
	a.Describe(&s.EndToEndEncryption, "Whether video streams use end-to-end encryption.")
	a.Describe(&s.CloudStorage, "Whether recordings go to the configured cloud storage.")
}

// VideoCapability configures the Video API capability.
type VideoCapability struct {
	Webhooks *VideoWebhooks `pulumi:"webhooks,optional"`
	Storage  *VideoStorage  `pulumi:"storage,optional"`
}

func (v *VideoCapability) Annotate(a infer.Annotator) {
	a.Describe(&v.Webhooks, "Webhooks that receive Video API events.")
	a.Describe(&v.Storage, "Recording storage and encryption settings.")
}

// NetworkApisCapability configures the Network APIs capability (SIM Swap, Number Verification, ...).
type NetworkApisCapability struct {
	NetworkApplicationID string  `pulumi:"networkApplicationId"`
	RedirectURI          *string `pulumi:"redirectUri,optional"`
}

func (n *NetworkApisCapability) Annotate(a infer.Annotator) {
	a.Describe(&n.NetworkApplicationID, "The Network APIs application ID.")
	a.Describe(&n.RedirectURI, "Redirect URI for the Network APIs OAuth flow.")
}

// --- wire shapes ---

type wireVideoHook struct {
	Address string `json:"address"`
	Active  *bool  `json:"active,omitempty"`
}

type wireVideoStorage struct {
	ServerSideEncryption *bool `json:"server_side_encryption,omitempty"`
	EndToEndEncryption   *bool `json:"end_to_end_encryption,omitempty"`
	CloudStorage         *bool `json:"cloud_storage,omitempty"`
}

type wireVideo struct {
	Webhooks map[string]wireVideoHook `json:"webhooks,omitempty"`
	Storage  *wireVideoStorage        `json:"storage,omitempty"`
}

type wireNetworkAPIs struct {
	NetworkApplicationID string  `json:"network_application_id,omitempty"`
	RedirectURI          *string `json:"redirect_uri,omitempty"`
}

const (
	capVideo      = "video"
	capNetworkAPI = "network_apis"
)

// videoHookSlots pairs each wire webhook name with its field.
func videoHookSlots(w *VideoWebhooks) []struct {
	key  string
	slot **VideoWebhook
} {
	return []struct {
		key  string
		slot **VideoWebhook
	}{
		{"archive_status", &w.ArchiveStatus}, {"broadcast_status", &w.BroadcastStatus},
		{"captions_status", &w.CaptionsStatus}, {"connection_created", &w.ConnectionCreated},
		{"connection_destroyed", &w.ConnectionDestroyed}, {"render_status", &w.RenderStatus},
		{"session_created", &w.SessionCreated}, {"session_destroyed", &w.SessionDestroyed},
		{"session_notification", &w.SessionNotification}, {"sip_call_created", &w.SipCallCreated},
		{"sip_call_destroyed", &w.SipCallDestroyed}, {"sip_call_mute_forced", &w.SipCallMuteForced},
		{"sip_call_updated", &w.SipCallUpdated}, {"stream_created", &w.StreamCreated},
		{"stream_destroyed", &w.StreamDestroyed},
	}
}

func toWireVideo(v *VideoCapability) json.RawMessage {
	wv := wireVideo{}
	if v.Webhooks != nil {
		for _, s := range videoHookSlots(v.Webhooks) {
			if h := *s.slot; h != nil {
				if wv.Webhooks == nil {
					wv.Webhooks = map[string]wireVideoHook{}
				}
				wv.Webhooks[s.key] = wireVideoHook{Address: h.Address, Active: h.Active}
			}
		}
	}
	if st := v.Storage; st != nil {
		wv.Storage = &wireVideoStorage{ServerSideEncryption: st.ServerSideEncryption,
			EndToEndEncryption: st.EndToEndEncryption, CloudStorage: st.CloudStorage}
	}
	b, _ := json.Marshal(wv)
	return b
}

func toWireNetworkAPIs(n *NetworkApisCapability) json.RawMessage {
	b, _ := json.Marshal(wireNetworkAPIs{NetworkApplicationID: n.NetworkApplicationID, RedirectURI: n.RedirectURI})
	return b
}

// withDeclaredExtras adds the declared video and network_apis capabilities to the
// wire body. They travel in Extra so an undeclared one can still be replayed from
// the server on update.
func withDeclaredExtras(w *wireApplication, a ApplicationArgs) {
	if a.Video == nil && a.NetworkApis == nil {
		return
	}
	if w.Capabilities == nil {
		w.Capabilities = &wireCapabilities{}
	}
	if w.Capabilities.Extra == nil {
		w.Capabilities.Extra = map[string]json.RawMessage{}
	}
	if a.Video != nil {
		w.Capabilities.Extra[capVideo] = toWireVideo(a.Video)
	}
	if a.NetworkApis != nil {
		w.Capabilities.Extra[capNetworkAPI] = toWireNetworkAPIs(a.NetworkApis)
	}
}

// preserveUnmanagedExtras replays capabilities held by the server that the program
// does not declare. A video or network_apis capability that an earlier run of the
// program declared (old) and this one dropped is removed instead, so deleting the
// block disables the capability.
func preserveUnmanagedExtras(body *wireApplication, cur *wireApplication, old ApplicationArgs) {
	if cur == nil || cur.Capabilities == nil || len(cur.Capabilities.Extra) == 0 {
		return
	}
	if body.Capabilities == nil {
		body.Capabilities = &wireCapabilities{}
	}
	if body.Capabilities.Extra == nil {
		body.Capabilities.Extra = map[string]json.RawMessage{}
	}
	for k, v := range cur.Capabilities.Extra {
		if _, declared := body.Capabilities.Extra[k]; declared {
			continue
		}
		if (k == capVideo && old.Video != nil) || (k == capNetworkAPI && old.NetworkApis != nil) {
			continue
		}
		body.Capabilities.Extra[k] = v
	}
}

// observeVideo projects the server's video capability onto the declared shape.
// It runs only when the program declares video, so a capability configured
// outside Pulumi never shows up as drift.
func observeVideo(prev *VideoCapability, raw json.RawMessage) *VideoCapability {
	var wv wireVideo
	if err := json.Unmarshal(raw, &wv); err != nil {
		return prev
	}
	out := &VideoCapability{}
	if len(wv.Webhooks) > 0 {
		hooks := &VideoWebhooks{}
		var pw *VideoWebhooks
		if prev != nil {
			pw = prev.Webhooks
		}
		if pw == nil {
			pw = &VideoWebhooks{}
		}
		prevSlots := videoHookSlots(pw)
		for i, s := range videoHookSlots(hooks) {
			h, ok := wv.Webhooks[s.key]
			if !ok {
				continue
			}
			oh := &VideoWebhook{Address: h.Address}
			if *prevSlots[i].slot == nil || (*prevSlots[i].slot).Active != nil {
				oh.Active = h.Active
			}
			*s.slot = oh
		}
		out.Webhooks = hooks
	}
	if prev != nil && prev.Storage != nil && wv.Storage != nil {
		st := &VideoStorage{}
		if prev.Storage.ServerSideEncryption != nil {
			st.ServerSideEncryption = wv.Storage.ServerSideEncryption
		}
		if prev.Storage.EndToEndEncryption != nil {
			st.EndToEndEncryption = wv.Storage.EndToEndEncryption
		}
		if prev.Storage.CloudStorage != nil {
			st.CloudStorage = wv.Storage.CloudStorage
		}
		out.Storage = st
	} else if prev != nil && prev.Storage != nil {
		out.Storage = prev.Storage
	}
	return out
}

func observeNetworkAPIs(prev *NetworkApisCapability, raw json.RawMessage) *NetworkApisCapability {
	var wn wireNetworkAPIs
	if err := json.Unmarshal(raw, &wn); err != nil {
		return prev
	}
	out := &NetworkApisCapability{NetworkApplicationID: wn.NetworkApplicationID}
	if prev != nil && prev.RedirectURI != nil {
		out.RedirectURI = wn.RedirectURI
	}
	return out
}

func checkVideoWebhooks(failures *[]p.CheckFailure, v *VideoCapability) {
	if v == nil || v.Webhooks == nil {
		return
	}
	for _, s := range videoHookSlots(v.Webhooks) {
		if h := *s.slot; h != nil && strings.TrimSpace(h.Address) == "" {
			*failures = append(*failures, p.CheckFailure{Property: "video.webhooks." + s.key + ".address",
				Reason: "address must not be empty"})
		}
	}
}
