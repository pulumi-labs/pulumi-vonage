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
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/stretchr/testify/require"
)

func appInputs() map[string]any {
	return map[string]any{
		"name": "demo",
		"voice": map[string]any{
			"answerUrl": map[string]any{"address": "https://example.com/answer", "httpMethod": "GET"},
			"eventUrl":  map[string]any{"address": "https://example.com/event", "httpMethod": "POST"},
			"region":    "eu-west",
		},
		"messages": map[string]any{
			"inboundUrl": map[string]any{"address": "https://example.com/in", "httpMethod": "POST"},
			"statusUrl":  map[string]any{"address": "https://example.com/status", "httpMethod": "POST"},
		},
	}
}

func TestApplication_Lifecycle(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	urn := vonageURN("Application", "app")

	// Preview does not call the API.
	cr, err := s.Create(pgp.CreateRequest{Urn: urn, Properties: pm(appInputs()), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, m.callLog())
	_ = cr

	cr, err = s.Create(pgp.CreateRequest{Urn: urn, Properties: pm(appInputs())})
	require.NoError(t, err)
	require.Equal(t, "app-1", cr.ID)
	require.Equal(t, "app-1", str(cr.Properties, "applicationId"))
	require.Equal(t, "PUB-app-1", str(cr.Properties, "applicationPublicKey"))
	priv, ok := cr.Properties.GetOk("privateKey")
	require.True(t, ok)
	require.Equal(t, "PRIV-app-1", priv.AsString())

	// Wire contract: snake_case capability payload, upper-case method.
	sent := m.apps["app-1"]
	caps := sent["capabilities"].(map[string]any)
	voice := caps["voice"].(map[string]any)
	hooks := voice["webhooks"].(map[string]any)
	require.Equal(t, "GET", hooks["answer_url"].(map[string]any)["http_method"])
	require.Equal(t, "eu-west", voice["region"])
	require.Contains(t, caps, "messages")
	require.NotContains(t, caps, "rtc")

	// Read: private key survives from state; no spurious diff in observed inputs.
	rr, err := s.Read(pgp.ReadRequest{ID: "app-1", Urn: urn, Properties: cr.Properties, Inputs: pm(appInputs())})
	require.NoError(t, err)
	require.Equal(t, "app-1", rr.ID)
	require.Equal(t, "PRIV-app-1", str(rr.Properties, "privateKey"))
	require.Equal(t, "demo", str(rr.Inputs, "name"))
	vo, _ := rr.Inputs.GetOk("voice")
	require.True(t, vo.IsMap())
	_, hasRtc := rr.Inputs.GetOk("rtc")
	require.False(t, hasRtc)

	// Update: add rtc, drop messages; whole config is replaced.
	in2 := appInputs()
	delete(in2, "messages")
	in2["rtc"] = map[string]any{"eventUrl": map[string]any{"address": "https://example.com/rtc"}}
	ur, err := s.Update(pgp.UpdateRequest{ID: "app-1", Urn: urn, State: cr.Properties, Inputs: pm(in2), OldInputs: pm(appInputs())})
	require.NoError(t, err)
	require.Equal(t, "PUB-app-1", str(ur.Properties, "applicationPublicKey"))
	require.Equal(t, "PRIV-app-1", str(ur.Properties, "privateKey"), "create-only key is carried across updates")
	caps = m.apps["app-1"]["capabilities"].(map[string]any)
	require.Contains(t, caps, "rtc")
	require.NotContains(t, caps, "messages")

	// Delete, then Read reports gone.
	require.NoError(t, s.Delete(pgp.DeleteRequest{ID: "app-1", Urn: urn, Properties: ur.Properties}))
	rr, err = s.Read(pgp.ReadRequest{ID: "app-1", Urn: urn, Properties: ur.Properties})
	require.NoError(t, err)
	require.Empty(t, rr.ID)
	require.True(t, m.authSeen)
}

func TestApplication_BringYourOwnPublicKeyHasNoPrivateKey(t *testing.T) {
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	in := map[string]any{"name": "byok", "publicKey": "-----BEGIN PUBLIC KEY-----\nabc\n-----END PUBLIC KEY-----"}
	cr, err := s.Create(pgp.CreateRequest{Urn: vonageURN("Application", "byok"), Properties: pm(in)})
	require.NoError(t, err)
	_, has := cr.Properties.GetOk("privateKey")
	require.False(t, has)
	require.Contains(t, str(cr.Properties, "applicationPublicKey"), "BEGIN PUBLIC KEY")
}

func TestApplication_CheckRejectsBadInputs(t *testing.T) {
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	cases := map[string]map[string]any{
		"empty name":      {"name": " "},
		"bad region":      {"name": "x", "voice": map[string]any{"region": "mars"}},
		"messages GET":    {"name": "x", "messages": map[string]any{"inboundUrl": map[string]any{"address": "https://e/x", "httpMethod": "GET"}}},
		"bad ttl":         {"name": "x", "voice": map[string]any{"conversationsTtl": 99999}},
		"socket timeout":  {"name": "x", "voice": map[string]any{"answerUrl": map[string]any{"address": "https://e/x", "socketTimeout": 5}}},
		"empty address":   {"name": "x", "rtc": map[string]any{"eventUrl": map[string]any{"address": ""}}},
		"bad http method": {"name": "x", "voice": map[string]any{"eventUrl": map[string]any{"address": "https://e/x", "httpMethod": "PUT"}}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := s.Check(pgp.CheckRequest{Urn: vonageURN("Application", "x"), Inputs: pm(in)})
			require.NoError(t, err)
			require.NotEmpty(t, r.Failures)
		})
	}
	r, err := s.Check(pgp.CheckRequest{Urn: vonageURN("Application", "ok"), Inputs: pm(appInputs())})
	require.NoError(t, err)
	require.Empty(t, r.Failures)
}

func TestApplication_ReadWithServerDefaultsDoesNotSurfaceUnmanagedFields(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	in := map[string]any{"name": "d", "voice": map[string]any{"eventUrl": map[string]any{"address": "https://e/x"}}}
	cr, err := s.Create(pgp.CreateRequest{Urn: vonageURN("Application", "d"), Properties: pm(in)})
	require.NoError(t, err)
	// Server fills defaults the user never declared.
	v := m.apps["app-1"]["capabilities"].(map[string]any)["voice"].(map[string]any)
	v["signed_callbacks"] = true
	v["conversations_ttl"] = float64(48)
	v["webhooks"].(map[string]any)["event_url"].(map[string]any)["http_method"] = "POST"
	rr, err := s.Read(pgp.ReadRequest{ID: "app-1", Urn: vonageURN("Application", "d"), Properties: cr.Properties, Inputs: pm(in)})
	require.NoError(t, err)
	vo, _ := rr.Inputs.GetOk("voice")
	vm := vo.AsMap()
	_, hasTTL := vm.GetOk("conversationsTtl")
	_, hasSigned := vm.GetOk("signedCallbacks")
	require.False(t, hasTTL)
	require.False(t, hasSigned)
	ev, _ := vm.GetOk("eventUrl")
	_, hasMethod := ev.AsMap().GetOk("httpMethod")
	require.False(t, hasMethod, "unmanaged httpMethod must not be adopted into inputs")
}

func TestClient_RetriesTransientGetButNotPost(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	m.fail5xx = 2
	r, err := s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getApplication", Args: pm(map[string]any{"name": "nobody"})})
	require.Error(t, err, "no such app, but only after the 503s were retried: %v", r)
	require.Contains(t, err.Error(), "no application named")
	require.Equal(t, 0, m.fail5xx, "both 503s consumed by retries")
}
