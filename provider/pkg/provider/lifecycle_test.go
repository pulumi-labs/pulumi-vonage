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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

// These tests drive every resource through the real provider RPC surface
// (integration.Server) against the in-memory fake in mock_test.go. The fake follows
// the documented Vonage semantics, not the provider's assumptions: messages.version
// and the private key are never returned by GET, /account/numbers never returns
// voiceStatusCallback, application PUT replaces the whole configuration, and an
// account holds at most two API secrets.

type vlc struct {
	t      *testing.T
	s      integration.Server
	m      *mockVonage
	urn    resource.URN
	srvURL string
}

func newVLC(t *testing.T, typ, name string) *vlc {
	t.Helper()
	m, srv := newMock(t)
	return &vlc{t: t, s: newServerFor(t, srv.URL), m: m, urn: vonageURN(typ, name), srvURL: srv.URL}
}

// clone deep-copies a map[string]any through JSON so tests can mutate variants.
func clone(in map[string]any) map[string]any {
	b, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

func (l *vlc) check(in map[string]any) property.Map {
	l.t.Helper()
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(in)})
	require.NoError(l.t, err)
	require.Empty(l.t, chk.Failures)
	return chk.Inputs
}

func (l *vlc) create(in map[string]any) pgp.CreateResponse {
	l.t.Helper()
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in)})
	require.NoError(l.t, err)
	require.NotEmpty(l.t, c.ID)
	return c
}

func (l *vlc) diff(id string, state property.Map, inputs, old property.Map) pgp.DiffResponse {
	l.t.Helper()
	d, err := l.s.Diff(pgp.DiffRequest{ID: id, Urn: l.urn, State: state, Inputs: inputs, OldInputs: old})
	require.NoError(l.t, err)
	return d
}

func (l *vlc) update(id string, state property.Map, inputs, old property.Map) pgp.UpdateResponse {
	l.t.Helper()
	u, err := l.s.Update(pgp.UpdateRequest{ID: id, Urn: l.urn, State: state, Inputs: inputs, OldInputs: old})
	require.NoError(l.t, err)
	return u
}

func (l *vlc) read(id string, state, inputs property.Map) pgp.ReadResponse {
	l.t.Helper()
	r, err := l.s.Read(pgp.ReadRequest{ID: id, Urn: l.urn, Properties: state, Inputs: inputs})
	require.NoError(l.t, err)
	return r
}

func (l *vlc) del(id string, state property.Map) {
	l.t.Helper()
	require.NoError(l.t, l.s.Delete(pgp.DeleteRequest{ID: id, Urn: l.urn, Properties: state}))
}

func noChanges(t *testing.T, d pgp.DiffResponse) {
	t.Helper()
	require.False(t, d.HasChanges, "unexpected diff: %+v", d.DetailedDiff)
}

func emptyMap() property.Map { return pm(map[string]any{}) }

// nested reads a nested string property from a property map.
func nested(t *testing.T, m property.Map, path ...string) (string, bool) {
	t.Helper()
	cur := m
	for i, p := range path {
		v, ok := cur.GetOk(p)
		if !ok {
			return "", false
		}
		if i == len(path)-1 {
			if !v.IsString() {
				return "", false
			}
			return v.AsString(), true
		}
		require.True(t, v.IsMap(), "%v is not a map", p)
		cur = v.AsMap()
	}
	return "", false
}

func (m *mockVonage) app(id string) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return clone(m.apps[id])
}

func (m *mockVonage) mutate(f func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f()
}

func (m *mockVonage) countCalls(call string) int {
	n := 0
	for _, c := range m.callLog() {
		if c == call {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Application
// ---------------------------------------------------------------------------

func fullAppInputs() map[string]any {
	in := appInputs()
	in["messages"].(map[string]any)["version"] = "v1"
	in["improveAi"] = false
	return in
}

func TestApplication_FullLifecycle(t *testing.T) {
	l := newVLC(t, "Application", "app")
	l.m.fillDefaults = true
	in := fullAppInputs()
	c := l.create(in)
	require.Equal(t, "app-1", c.ID)

	// Idempotent Diff right after create.
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// Refresh: the server fills defaults and never returns messages.version, yet the program
	// stays clean and the declared messages.version survives.
	rd := l.read(c.ID, c.Properties, pm(in))
	v, ok := nested(t, rd.Inputs, "messages", "version")
	require.True(t, ok, "messages.version is write-only and must survive a refresh")
	require.Equal(t, "v1", v)
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))
	require.Equal(t, "PRIV-app-1", str(rd.Properties, "privateKey"))

	// Update: every updatable property reaches the API in one full PUT.
	in2 := clone(in)
	in2["name"] = "demo2"
	in2["voice"].(map[string]any)["region"] = "na-east"
	in2["voice"].(map[string]any)["answerUrl"].(map[string]any)["address"] = "https://example.com/answer2"
	in2["messages"].(map[string]any)["inboundUrl"].(map[string]any)["address"] = "https://example.com/in2"
	in2["messages"].(map[string]any)["version"] = "v0.1"
	in2["rtc"] = map[string]any{"eventUrl": map[string]any{"address": "https://example.com/rtc"}, "signedCallbacks": true}
	in2["verify"] = map[string]any{"statusUrl": map[string]any{"address": "https://example.com/verify"}, "version": "v2"}
	in2["meetings"] = map[string]any{"recordingChanged": map[string]any{"address": "https://example.com/rec"}}
	in2["vbc"] = true
	in2["improveAi"] = true
	d := l.diff(c.ID, rd.Properties, pm(in2), pm(in))
	require.True(t, d.HasChanges)
	for _, k := range []string{"name", "voice", "messages", "rtc", "verify", "meetings", "vbc", "improveAi"} {
		found := false
		for dk := range d.DetailedDiff {
			if dk == k || len(dk) > len(k) && dk[:len(k)+1] == k+"." {
				found = true
			}
		}
		require.True(t, found, "diff must report %s: %+v", k, d.DetailedDiff)
	}
	ur := l.update(c.ID, rd.Properties, pm(in2), pm(in))
	app := l.m.app(c.ID)
	require.Equal(t, "demo2", app["name"])
	caps := app["capabilities"].(map[string]any)
	voice := caps["voice"].(map[string]any)
	require.Equal(t, "na-east", voice["region"])
	require.Equal(t, "https://example.com/answer2", voice["webhooks"].(map[string]any)["answer_url"].(map[string]any)["address"])
	msgs := caps["messages"].(map[string]any)
	require.Equal(t, "v0.1", msgs["version"])
	require.Equal(t, "https://example.com/in2", msgs["webhooks"].(map[string]any)["inbound_url"].(map[string]any)["address"])
	require.Equal(t, "https://example.com/rtc", caps["rtc"].(map[string]any)["webhooks"].(map[string]any)["event_url"].(map[string]any)["address"])
	require.Equal(t, "v2", caps["verify"].(map[string]any)["version"])
	require.Equal(t, "https://example.com/rec", caps["meetings"].(map[string]any)["webhooks"].(map[string]any)["recording_changed"].(map[string]any)["address"])
	require.Contains(t, caps, "vbc")
	require.Equal(t, true, app["privacy"].(map[string]any)["improve_ai"])
	require.Equal(t, "PRIV-app-1", str(ur.Properties, "privateKey"))
	rd = l.read(c.ID, ur.Properties, pm(in2))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in2), rd.Inputs))

	// Drift made outside Pulumi shows up in the refreshed inputs and in the next Diff.
	l.m.mutate(func() {
		l.m.apps[c.ID]["name"] = "renamed"
		l.m.apps[c.ID]["capabilities"].(map[string]any)["voice"].(map[string]any)["webhooks"].(map[string]any)["answer_url"].(map[string]any)["address"] = "https://evil.example/answer"
	})
	rd = l.read(c.ID, ur.Properties, pm(in2))
	require.Equal(t, "renamed", str(rd.Inputs, "name"))
	dv, _ := nested(t, rd.Inputs, "voice", "answerUrl", "address")
	require.Equal(t, "https://evil.example/answer", dv)
	dd := l.diff(c.ID, rd.Properties, pm(in2), rd.Inputs)
	require.True(t, dd.HasChanges)
	require.Contains(t, dd.DetailedDiff, "name")

	// Import by ID alone adopts the server view; a program equal to the imported inputs is clean.
	imp := l.read(c.ID, emptyMap(), emptyMap())
	require.Equal(t, c.ID, imp.ID)
	require.Equal(t, c.ID, str(imp.Properties, "applicationId"))
	require.Equal(t, "renamed", str(imp.Inputs, "name"))
	_, hasPriv := imp.Properties.GetOk("privateKey")
	require.False(t, hasPriv, "Vonage never returns the private key again")
	noChanges(t, l.diff(imp.ID, imp.Properties, imp.Inputs, imp.Inputs))

	// Deleted outside Pulumi: Read reports gone, Delete is quiet, also when repeated.
	l.del(c.ID, ur.Properties)
	require.Empty(t, l.read(c.ID, ur.Properties, pm(in2)).ID)
	l.del(c.ID, ur.Properties)
	require.Empty(t, l.read(c.ID, emptyMap(), emptyMap()).ID)
}

func TestApplication_NestedVersionIsDiffed(t *testing.T) {
	l := newVLC(t, "Application", "app")
	in := fullAppInputs()
	in["verify"] = map[string]any{"version": "v2"}
	c := l.create(in)
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))
	in2 := clone(in)
	in2["messages"].(map[string]any)["version"] = "v0.1"
	d := l.diff(c.ID, c.Properties, pm(in2), pm(in))
	require.Equal(t, pgp.Update, diffKinds(d)["messages.version"])
	in3 := clone(in)
	in3["verify"].(map[string]any)["version"] = "v3"
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, c.Properties, pm(in3), pm(in)))["verify.version"])
}

// Bug 3: messages.version is write-only. After a refresh the next preview must not show an update.
func TestApplication_RefreshKeepsWriteOnlyMessagesVersion(t *testing.T) {
	l := newVLC(t, "Application", "app")
	in := fullAppInputs()
	c := l.create(in)
	require.NotContains(t, stripWriteOnly(l.m.app(c.ID))["capabilities"].(map[string]any)["messages"], "version",
		"sanity: the fake omits messages.version on GET like the real API")
	got, err := l.s.Read(pgp.ReadRequest{ID: c.ID, Urn: l.urn, Properties: c.Properties, Inputs: pm(in)})
	require.NoError(t, err)
	v, _ := nested(t, got.Inputs, "messages", "version")
	require.Equal(t, "v1", v)
	v, _ = nested(t, got.Properties, "messages", "version")
	require.Equal(t, "v1", v)
	noChanges(t, l.diff(c.ID, got.Properties, pm(in), got.Inputs))
}

// Bug 4: a declared publicKey is refreshed, so a key rotated outside Pulumi is drift.
func TestApplication_PublicKeyDriftIsDetectedAndRestored(t *testing.T) {
	l := newVLC(t, "Application", "app")
	pem1 := "-----BEGIN PUBLIC KEY-----\nAAA\n-----END PUBLIC KEY-----"
	in := map[string]any{"name": "byok", "publicKey": pem1}
	c := l.create(in)
	require.Equal(t, pem1, str(c.Properties, "applicationPublicKey"))

	// A trailing newline from the server is not drift.
	l.m.mutate(func() { l.m.apps[c.ID]["keys"] = map[string]any{"public_key": pem1 + "\n"} })
	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, pem1, str(rd.Inputs, "publicKey"))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// A different key is drift, and Update restores the declared one.
	pem2 := "-----BEGIN PUBLIC KEY-----\nBBB\n-----END PUBLIC KEY-----"
	l.m.mutate(func() { l.m.apps[c.ID]["keys"] = map[string]any{"public_key": pem2} })
	rd = l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, pem2, str(rd.Inputs, "publicKey"))
	require.Equal(t, pem2, str(rd.Properties, "applicationPublicKey"))
	d := l.diff(c.ID, rd.Properties, pm(in), rd.Inputs)
	require.Equal(t, pgp.Update, diffKinds(d)["publicKey"])
	ur := l.update(c.ID, rd.Properties, pm(in), rd.Inputs)
	require.Equal(t, pem1, str(ur.Properties, "applicationPublicKey"))
	require.Equal(t, pem1, l.m.app(c.ID)["keys"].(map[string]any)["public_key"])

	// Without a declared key the registered key is only an output; no input appears.
	l2 := newVLC(t, "Application", "gen")
	c2 := l2.create(map[string]any{"name": "gen"})
	rd2 := l2.read(c2.ID, c2.Properties, pm(map[string]any{"name": "gen"}))
	_, has := rd2.Inputs.GetOk("publicKey")
	require.False(t, has)
	require.Equal(t, "PUB-app-1", str(rd2.Properties, "applicationPublicKey"))
}

// Bug 6: Update is a full PUT, so capabilities the provider does not model must be replayed.
func TestApplication_UpdatePreservesUnmodelledCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   map[string]any
	}{
		{"with modelled capabilities", appInputs()},
		{"with no declared capabilities", map[string]any{"name": "bare"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newVLC(t, "Application", "app")
			c := l.create(tc.in)
			l.m.mutate(func() {
				caps, _ := l.m.apps[c.ID]["capabilities"].(map[string]any)
				if caps == nil {
					caps = map[string]any{}
					l.m.apps[c.ID]["capabilities"] = caps
				}
				caps["video"] = map[string]any{"webhooks": map[string]any{"connection_url": map[string]any{"address": "https://example.com/video", "http_method": "POST"}}}
				caps["network_apis"] = map[string]any{"network_application_id": "na-1"}
			})
			// The refresh does not show the unmodelled capabilities as drift.
			rd := l.read(c.ID, c.Properties, pm(tc.in))
			noChanges(t, l.diff(c.ID, rd.Properties, pm(tc.in), rd.Inputs))

			in2 := clone(tc.in)
			in2["name"] = "renamed"
			l.update(c.ID, rd.Properties, pm(in2), rd.Inputs)
			caps := l.m.app(c.ID)["capabilities"].(map[string]any)
			require.Contains(t, caps, "video", "video configured outside Pulumi must survive an update")
			require.Equal(t, "na-1", caps["network_apis"].(map[string]any)["network_application_id"])
			require.Equal(t, "https://example.com/video",
				caps["video"].(map[string]any)["webhooks"].(map[string]any)["connection_url"].(map[string]any)["address"])
			if _, ok := tc.in["voice"]; ok {
				require.Contains(t, caps, "voice")
			}
		})
	}
}

func TestApplication_UpdateOfMissingApplicationFailsClearly(t *testing.T) {
	l := newVLC(t, "Application", "app")
	in := appInputs()
	c := l.create(in)
	l.del(c.ID, c.Properties)
	in2 := clone(in)
	in2["name"] = "again"
	_, err := l.s.Update(pgp.UpdateRequest{ID: c.ID, Urn: l.urn, State: c.Properties, Inputs: pm(in2), OldInputs: pm(in)})
	require.ErrorContains(t, err, "no longer exists")
	require.Equal(t, 0, l.m.countCalls("PUT /v2/applications/"+c.ID))
}

func TestClient_ListApplicationsWalksEveryPage(t *testing.T) {
	m, srv := newMock(t)
	m.mutate(func() {
		for i := 1; i <= 250; i++ {
			id := fmt.Sprintf("app-%d", i)
			m.apps[id] = map[string]any{"id": id, "name": fmt.Sprintf("n%d", i), "keys": map[string]any{"public_key": "k"}}
		}
		m.nextApp = 250
	})
	c := newClient("key12345", "Secret123", srv.URL, srv.URL)
	apps, err := c.ListApplications(t.Context())
	require.NoError(t, err)
	require.Len(t, apps, 250)
	require.Equal(t, 3, m.countCalls("GET /v2/applications"), "250 applications need three pages of 100")

	// The by-name lookup finds an application on the last page.
	s := newServerFor(t, srv.URL)
	r, err := s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getApplication", Args: pm(map[string]any{"name": "n250"})})
	require.NoError(t, err)
	require.Equal(t, "app-250", str(r.Return, "applicationId"))
}

// ---------------------------------------------------------------------------
// Number
// ---------------------------------------------------------------------------

func buyInputs() map[string]any {
	return map[string]any{"country": "US", "msisdn": "14155550100", "acquisition": "buy", "cancelOnDelete": true,
		"moHttpUrl": "https://example.com/sms"}
}

func TestNumber_FullLifecycle(t *testing.T) {
	l := newVLC(t, "Number", "n")
	in := buyInputs()
	c := l.create(in)
	require.Equal(t, "US:14155550100", c.ID)
	require.Equal(t, "mobile-lvn", str(c.Properties, "type"))
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	key := "US:14155550100"
	// Update 1: moHttpUrl.
	in2 := clone(in)
	in2["moHttpUrl"] = "https://example.com/sms2"
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, rd.Properties, pm(in2), pm(in)))["moHttpUrl"])
	u := l.update(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, "https://example.com/sms2", l.m.numbers[key]["moHttpUrl"])

	// Update 2: voice routing plus status callback instead of the SMS webhook.
	in3 := clone(in2)
	delete(in3, "moHttpUrl")
	in3["voiceCallbackType"], in3["voiceCallbackValue"], in3["voiceStatusCallback"] = "tel", "14155550199", "https://example.com/vs"
	d := l.diff(c.ID, u.Properties, pm(in3), pm(in2))
	for _, k := range []string{"moHttpUrl", "voiceCallbackType", "voiceCallbackValue", "voiceStatusCallback"} {
		require.Equal(t, pgp.Update, diffKinds(d)[k], k)
	}
	u = l.update(c.ID, u.Properties, pm(in3), pm(in2))
	require.Equal(t, "", l.m.numbers[key]["moHttpUrl"])
	require.Equal(t, "tel", l.m.numbers[key]["voiceCallbackType"])
	require.Equal(t, "14155550199", l.m.numbers[key]["voiceCallbackValue"])
	require.Equal(t, "https://example.com/vs", l.m.numbers[key]["voiceStatusCallback"])
	rd = l.read(c.ID, u.Properties, pm(in3))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in3), rd.Inputs))

	// Update 3: link to an application, dropping the direct routing.
	in4 := clone(in3)
	for _, k := range []string{"voiceCallbackType", "voiceCallbackValue", "voiceStatusCallback"} {
		delete(in4, k)
	}
	in4["appId"] = "app-9"
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, rd.Properties, pm(in4), pm(in3)))["appId"])
	u = l.update(c.ID, rd.Properties, pm(in4), pm(in3))
	require.Equal(t, "app-9", l.m.numbers[key]["app_id"])
	require.Equal(t, "app-9", str(u.Properties, "appId"))
	require.Equal(t, "app", str(u.Properties, "effectiveVoiceCallbackType"))
	rd = l.read(c.ID, u.Properties, pm(in4))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in4), rd.Inputs))

	// Update 4: cancelOnDelete is provider-side only; no number API call is needed.
	in5 := clone(in4)
	in5["cancelOnDelete"] = false
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, rd.Properties, pm(in5), pm(in4)))["cancelOnDelete"])
	before := l.m.countCalls("POST /number/update")
	u = l.update(c.ID, rd.Properties, pm(in5), pm(in4))
	require.Equal(t, before, l.m.countCalls("POST /number/update"))
	require.Equal(t, false, u.Properties.Get("cancelOnDelete").AsBool())
	in6 := clone(in5)
	in6["cancelOnDelete"] = true
	u = l.update(c.ID, u.Properties, pm(in6), pm(in5))

	// Refresh detects drift of the managed routing.
	l.m.mutate(func() { l.m.numbers[key]["app_id"] = "app-other" })
	rd = l.read(c.ID, u.Properties, pm(in6))
	require.Equal(t, "app-other", str(rd.Inputs, "appId"))
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, rd.Properties, pm(in6), rd.Inputs))["appId"])

	// Import by ID alone, then no change for a program equal to the imported inputs.
	imp := l.read(c.ID, emptyMap(), emptyMap())
	require.Equal(t, c.ID, imp.ID)
	require.Equal(t, "US", str(imp.Inputs, "country"))
	require.Equal(t, "14155550100", str(imp.Inputs, "msisdn"))
	require.Equal(t, "adopt", str(imp.Inputs, "acquisition"))
	require.Equal(t, false, imp.Inputs.Get("cancelOnDelete").AsBool())
	noChanges(t, l.diff(imp.ID, imp.Properties, imp.Inputs, imp.Inputs))

	// Delete cancels (cancelOnDelete=true), and is quiet when the number is already gone.
	l.del(c.ID, u.Properties)
	require.NotContains(t, l.m.numbers, key)
	require.Equal(t, 1, l.m.countCalls("POST /number/cancel"))
	l.del(c.ID, u.Properties)
	require.Equal(t, 1, l.m.countCalls("POST /number/cancel"), "no second cancel for a number that is gone")
	require.Empty(t, l.read(c.ID, u.Properties, pm(in6)).ID)
	require.Empty(t, l.read(c.ID, emptyMap(), emptyMap()).ID)
}

// Bug 1: acquisition only matters at create time. Flipping it must never plan a replacement,
// because the replacement has the same ID and deleting the old resource would cancel the number.
func TestNumber_AcquisitionIsCreateOnly(t *testing.T) {
	l := newVLC(t, "Number", "n")
	l.m.setNumberRaw("US", "14155550100", nil)
	adopt := map[string]any{"country": "US", "msisdn": "14155550100", "acquisition": "adopt", "cancelOnDelete": true}
	c := l.create(adopt)
	buy := clone(adopt)
	buy["acquisition"] = "buy"

	// adopt -> buy and back: no diff at all, so nothing is replaced or cancelled.
	d := l.diff(c.ID, c.Properties, pm(buy), pm(adopt))
	noChanges(t, d)
	require.Empty(t, d.DetailedDiff)

	// An acquisition flip alongside a real change is an in-place update that never records the new mode.
	buy["moHttpUrl"] = "https://example.com/sms"
	d = l.diff(c.ID, c.Properties, pm(buy), pm(adopt))
	require.Equal(t, map[string]pgp.DiffKind{"moHttpUrl": pgp.Update}, diffKinds(d))
	u := l.update(c.ID, c.Properties, pm(buy), pm(adopt))
	require.Equal(t, "adopt", str(u.Properties, "acquisition"))
	require.Equal(t, 0, l.m.countCalls("POST /number/buy"))
	require.Equal(t, 0, l.m.countCalls("POST /number/cancel"))
	require.Contains(t, l.m.numbers, "US:14155550100")

	// An imported number in a program that says buy is clean as well (it used to plan a failing replace).
	imp := l.read(c.ID, emptyMap(), emptyMap())
	prog := map[string]any{"country": "US", "msisdn": "14155550100", "acquisition": "buy", "cancelOnDelete": false,
		"moHttpUrl": "https://example.com/sms"}
	noChanges(t, l.diff(imp.ID, imp.Properties, pm(prog), imp.Inputs))

	// country and msisdn still identify the number.
	other := clone(adopt)
	other["msisdn"] = "14155550101"
	require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, u.Properties, pm(other), pm(adopt)))["msisdn"])
	other = clone(adopt)
	other["country"] = "CA"
	require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, u.Properties, pm(other), pm(adopt)))["country"])
}

// Bug 7: an import ID must carry the country.
func TestNumber_ImportRejectsMalformedIDs(t *testing.T) {
	l := newVLC(t, "Number", "n")
	l.m.setNumberRaw("US", "14155550100", nil)
	for _, id := range []string{"14155550100", "us:14155550100", "USA:14155550100", "US:", "US:+14155550100", ":14155550100", "US:abc"} {
		_, err := l.s.Read(pgp.ReadRequest{ID: id, Urn: l.urn, Properties: emptyMap()})
		require.ErrorContains(t, err, "invalid Number ID", id)
	}
	require.Equal(t, 0, l.m.countCalls("GET /account/numbers"), "a malformed ID must not reach the API")
	rr := l.read("US:14155550100", emptyMap(), emptyMap())
	require.Equal(t, "US:14155550100", rr.ID)
}

func TestNumber_CreateRoutingFailureKeepsTheNumber(t *testing.T) {
	l := newVLC(t, "Number", "n")
	l.m.inject["POST /number/update"] = 500
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(buyInputs())})
	require.Error(t, err)
	require.Equal(t, "US:14155550100", c.ID, "the purchased number must be tracked, not orphaned")
	require.NotNil(t, c.PartialState)
	require.Contains(t, l.m.numbers, "US:14155550100")
	require.Equal(t, 1, l.m.countCalls("POST /number/buy"))
}

// Bug 8: a read-back that fails after the purchase must not be swallowed.
func TestNumber_CreateReadBackFailureIsReportedWithPartialState(t *testing.T) {
	l := newVLC(t, "Number", "n")
	in := buyInputs()
	delete(in, "moHttpUrl")
	l.m.after["POST /number/buy"] = func() { l.m.inject["GET /account/numbers"] = 400 }
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in)})
	require.Error(t, err)
	require.Equal(t, "US:14155550100", c.ID)
	require.NotNil(t, c.PartialState)
	require.Contains(t, c.PartialState.Reasons[0], "reading it back")
	require.Contains(t, l.m.numbers, "US:14155550100")
}

func TestNumber_CreateNotYetListedIsReportedWithPartialState(t *testing.T) {
	l := newVLC(t, "Number", "n")
	in := buyInputs()
	delete(in, "moHttpUrl")
	// The purchase succeeds but the account listing does not show the number yet.
	l.m.after["POST /number/buy"] = func() {
		delete(l.m.numbers, "US:14155550100")
	}
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in)})
	require.Error(t, err)
	require.Equal(t, "US:14155550100", c.ID)
	require.NotNil(t, c.PartialState)
	require.Contains(t, c.PartialState.Reasons[0], "not yet listed")
}

// Bug 11: the billed purchase is never replayed after a 5xx.
func TestNumber_BuyIsNotReplayedOnServerError(t *testing.T) {
	l := newVLC(t, "Number", "n")
	l.m.inject["POST /number/buy"] = 503
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(buyInputs())})
	require.Error(t, err)
	require.Equal(t, 1, l.m.countCalls("POST /number/buy"), "a POST must never be replayed after a 5xx")

	// Idempotent reads are retried.
	l.m.inject = map[string]int{}
	l.m.fail5xx = 2
	_, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getApplication", Args: pm(map[string]any{"name": "nobody"})})
	require.Error(t, err)
	require.Equal(t, 3, l.m.countCalls("GET /v2/applications"))
}

func TestNumber_FindOwnedNumberWalksEveryPage(t *testing.T) {
	m, srv := newMock(t)
	m.mutate(func() {
		for i := 1; i <= 250; i++ {
			msisdn := fmt.Sprintf("199%06d", i)
			m.numbers["US:"+msisdn] = map[string]any{"country": "US", "msisdn": msisdn, "type": "mobile-lvn", "features": []any{"SMS"}}
		}
		// "99" is contained in every other number, so the pattern search returns 251 hits and
		// the exact number sorts onto the third page.
		m.numbers["US:99"] = map[string]any{"country": "US", "msisdn": "99", "type": "landline", "features": []any{"VOICE"}}
	})
	c := newClient("key12345", "Secret123", srv.URL, srv.URL)
	n, err := c.FindOwnedNumber(t.Context(), "US", "99")
	require.NoError(t, err)
	require.NotNil(t, n)
	require.Equal(t, "landline", n.Type)
	require.Equal(t, 3, m.countCalls("GET /account/numbers"))
}

func TestNumber_UpdateClearsRemovedRoutingAndReadSurfacesEffectiveValues(t *testing.T) {
	l := newVLC(t, "Number", "n")
	l.m.setNumberRaw("US", "14155550100", map[string]any{"app_id": "app-1", "voiceCallbackType": "app", "voiceCallbackValue": "app-1",
		"messagesCallbackType": "app", "messagesCallbackValue": "app-1"})
	in := map[string]any{"country": "US", "msisdn": "14155550100", "acquisition": "adopt", "cancelOnDelete": false, "appId": "app-1"}
	c := l.create(in)
	require.Equal(t, "app", str(c.Properties, "messagesCallbackType"))
	require.Equal(t, "app-1", str(c.Properties, "messagesCallbackValue"))
	require.Equal(t, "app", str(c.Properties, "effectiveVoiceCallbackType"))
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))
	// Removing appId unlinks the number on Vonage.
	in2 := clone(in)
	delete(in2, "appId")
	u := l.update(c.ID, c.Properties, pm(in2), pm(in))
	require.Equal(t, "", l.m.numbers["US:14155550100"]["app_id"])
	_, has := u.Properties.GetOk("appId")
	require.False(t, has)
}

// ---------------------------------------------------------------------------
// SubAccount
// ---------------------------------------------------------------------------

func TestSubAccount_FullLifecycle(t *testing.T) {
	l := newVLC(t, "SubAccount", "sub")
	in := map[string]any{"name": "team-a", "usePrimaryAccountBalance": true, "suspendOnDelete": true}
	c := l.create(in)
	require.Equal(t, "sub00001", c.ID)
	require.Equal(t, "GenSecret1", str(c.Properties, "generatedSecret"))
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))
	require.Equal(t, "GenSecret1", str(rd.Properties, "generatedSecret"))

	// Update: each updatable property reaches the API.
	in2 := map[string]any{"name": "team-b", "usePrimaryAccountBalance": false, "suspended": true, "suspendOnDelete": true}
	d := l.diff(c.ID, rd.Properties, pm(in2), pm(in))
	for _, k := range []string{"name", "usePrimaryAccountBalance", "suspended"} {
		require.Equal(t, pgp.Update, diffKinds(d)[k], k)
	}
	u := l.update(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, "team-b", l.m.subs[c.ID]["name"])
	require.Equal(t, false, l.m.subs[c.ID]["use_primary_account_balance"])
	require.Equal(t, true, l.m.subs[c.ID]["suspended"])
	rd = l.read(c.ID, u.Properties, pm(in2))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in2), rd.Inputs))

	// Bug 5: removing suspended (and the balance override) from the program restores the defaults on Vonage.
	in3 := map[string]any{"name": "team-b", "suspendOnDelete": true}
	d = l.diff(c.ID, rd.Properties, pm(in3), pm(in2))
	require.Equal(t, pgp.Update, diffKinds(d)["suspended"])
	require.Equal(t, pgp.Update, diffKinds(d)["usePrimaryAccountBalance"])
	u = l.update(c.ID, rd.Properties, pm(in3), pm(in2))
	require.Equal(t, false, l.m.subs[c.ID]["suspended"], "the subaccount must be unsuspended")
	require.Equal(t, true, l.m.subs[c.ID]["use_primary_account_balance"])
	rd = l.read(c.ID, u.Properties, pm(in3))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in3), rd.Inputs))
	_, hasSusp := rd.Inputs.GetOk("suspended")
	require.False(t, hasSusp, "a server default must not be written into the inputs")

	// Bug 5: a subaccount suspended outside Pulumi is visible after a refresh even when the program omits suspended.
	l.m.mutate(func() { l.m.subs[c.ID]["suspended"] = true })
	rd = l.read(c.ID, u.Properties, pm(in3))
	require.Equal(t, true, rd.Inputs.Get("suspended").AsBool())
	require.Equal(t, true, rd.Properties.Get("suspended").AsBool())
	d = l.diff(c.ID, rd.Properties, pm(in3), rd.Inputs)
	require.Equal(t, pgp.Update, diffKinds(d)["suspended"])
	u = l.update(c.ID, rd.Properties, pm(in3), rd.Inputs)
	require.Equal(t, false, l.m.subs[c.ID]["suspended"])

	// Bug 5: import adopts suspended and the balance override; the imported program is clean.
	l.m.mutate(func() {
		l.m.subs[c.ID]["suspended"] = true
		l.m.subs[c.ID]["use_primary_account_balance"] = false
	})
	imp := l.read(c.ID, emptyMap(), emptyMap())
	require.Equal(t, c.ID, imp.ID)
	require.Equal(t, "team-b", str(imp.Inputs, "name"))
	require.Equal(t, true, imp.Inputs.Get("suspended").AsBool())
	require.Equal(t, false, imp.Inputs.Get("usePrimaryAccountBalance").AsBool())
	require.Equal(t, "primaryAccountApiKey", "primaryAccountApiKey")
	require.Equal(t, "key12345", str(imp.Properties, "primaryAccountApiKey"))
	noChanges(t, l.diff(imp.ID, imp.Properties, imp.Inputs, imp.Inputs))

	// Delete with suspendOnDelete suspends and never deletes; a vanished subaccount is quiet.
	l.m.mutate(func() { l.m.subs[c.ID]["suspended"] = false })
	l.del(c.ID, u.Properties)
	require.Equal(t, true, l.m.subs[c.ID]["suspended"])
	l.m.mutate(func() { delete(l.m.subs, c.ID) })
	require.Empty(t, l.read(c.ID, u.Properties, pm(in3)).ID)
	require.Empty(t, l.read(c.ID, emptyMap(), emptyMap()).ID)
	l.del(c.ID, u.Properties) // 404 on suspend is success
	l.del(c.ID, u.Properties)
}

func TestSubAccount_CreateSuspendFailureKeepsTheSubaccount(t *testing.T) {
	l := newVLC(t, "SubAccount", "sub")
	l.m.inject["PATCH /accounts/key12345/subaccounts/sub00001"] = 500
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(map[string]any{"name": "team-a", "suspended": true})})
	require.Error(t, err)
	require.Equal(t, "sub00001", c.ID, "the created subaccount must be tracked, not orphaned")
	require.NotNil(t, c.PartialState)
	require.Equal(t, "GenSecret1", str(c.Properties, "generatedSecret"), "the one-time secret must be kept in the partial state")
	require.Contains(t, l.m.subs, "sub00001")
}

// ---------------------------------------------------------------------------
// ApiSecret
// ---------------------------------------------------------------------------

func TestApiSecret_FullLifecycle(t *testing.T) {
	l := newVLC(t, "ApiSecret", "sec")
	l.m.secrets["key12345"] = map[string]string{"orig": "2025-01-01T00:00:00Z"}
	in := map[string]any{"secret": "NewSecret9"}
	c := l.create(in)
	require.Equal(t, "key12345/sec-1", c.ID)
	require.Equal(t, "sec-1", str(c.Properties, "secretId"))
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))
	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, "NewSecret9", str(rd.Properties, "secret"))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// A subaccount-owned secret goes to the subaccount.
	l.m.mutate(func() { l.m.secrets["sub00001"] = map[string]string{"s0": "2025-01-01T00:00:00Z"} })
	cs := l.create(map[string]any{"secret": "SubSecret9", "accountKey": "sub00001"})
	require.Equal(t, "sub00001/sec-2", cs.ID)
	require.Equal(t, "sub00001", str(cs.Properties, "ownerKey"))

	// Import by ID alone adopts the secret in place; the first update records the value without API calls.
	imp := l.read("key12345/orig", emptyMap(), emptyMap())
	require.Equal(t, "orig", str(imp.Properties, "secretId"))
	adopted := l.diff(imp.ID, imp.Properties, pm(in), imp.Inputs)
	require.Equal(t, pgp.Update, diffKinds(adopted)["secret"])
	// Import of a subaccount-owned secret records the owner.
	impSub := l.read("sub00001/s0", emptyMap(), emptyMap())
	require.Equal(t, "sub00001", str(impSub.Inputs, "accountKey"))

	// Revoked outside Pulumi: Read reports gone, Delete is quiet (404), also when repeated.
	l.m.mutate(func() { delete(l.m.secrets["key12345"], "sec-1") })
	require.Empty(t, l.read(c.ID, c.Properties, pm(in)).ID)
	l.del(c.ID, c.Properties)
	l.del(c.ID, c.Properties)
	require.Equal(t, 2, l.m.countCalls("DELETE /accounts/key12345/secrets/sec-1"))
}

func TestApiSecret_ImportRejectsMalformedIDs(t *testing.T) {
	l := newVLC(t, "ApiSecret", "sec")
	for _, id := range []string{"orig", "/orig", "key12345/"} {
		_, err := l.s.Read(pgp.ReadRequest{ID: id, Urn: l.urn, Properties: emptyMap()})
		require.ErrorContains(t, err, "invalid ApiSecret ID", id)
	}
	require.Empty(t, l.m.callLog(), "a malformed ID must not reach the API")
}

// Bug 2: at the two-secret limit, replacing the secret must revoke the old one first.
func TestApiSecret_RotationAtTheLimitDeletesBeforeReplacing(t *testing.T) {
	l := newVLC(t, "ApiSecret", "sec")
	l.m.secrets["key12345"] = map[string]string{"orig": "2025-01-01T00:00:00Z"}
	in := map[string]any{"secret": "BlueSecret9"}
	c := l.create(in) // the account now holds two secrets
	next := map[string]any{"secret": "GreenSecret9"}

	d := l.diff(c.ID, c.Properties, pm(next), pm(in))
	require.Equal(t, pgp.UpdateReplace, diffKinds(d)["secret"])
	require.True(t, d.DeleteBeforeReplace, "two secrets means create-then-revoke would hit the limit")

	// Prove the point: create-first fails against the limit, delete-first succeeds.
	_, err := l.s.Create(pgp.CreateRequest{Urn: vonageURN("ApiSecret", "green"), Properties: l.check(next)})
	require.Error(t, err)
	l.del(c.ID, c.Properties)
	green, err := l.s.Create(pgp.CreateRequest{Urn: vonageURN("ApiSecret", "green"), Properties: l.check(next)})
	require.NoError(t, err)
	require.Equal(t, "GreenSecret9", str(green.Properties, "secret"))
	require.Contains(t, l.m.secrets["key12345"], "orig", "the original secret keeps working throughout")

	// With a single secret (only the managed one) the old one cannot be revoked first, so keep the default order.
	l2 := newVLC(t, "ApiSecret", "sec")
	c2 := l2.create(in)
	d2 := l2.diff(c2.ID, c2.Properties, pm(next), pm(in))
	require.Equal(t, pgp.UpdateReplace, diffKinds(d2)["secret"])
	require.False(t, d2.DeleteBeforeReplace)
	green2, err := l2.s.Create(pgp.CreateRequest{Urn: vonageURN("ApiSecret", "green"), Properties: l2.check(next)})
	require.NoError(t, err)
	l2.del(c2.ID, c2.Properties)
	require.Contains(t, l2.m.secrets["key12345"], "sec-2")
	_ = green2

	// A different owner is a plain replacement (its own limit); an unchanged program has no diff at all.
	d3 := l.diff(green.ID, green.Properties, pm(map[string]any{"secret": "GreenSecret9", "accountKey": "sub00001"}), pm(next))
	require.Equal(t, pgp.UpdateReplace, diffKinds(d3)["accountKey"])
	require.False(t, d3.DeleteBeforeReplace)
	noChanges(t, l.diff(green.ID, green.Properties, pm(next), pm(next)))
}

// A buy that fails ambiguously (5xx) after the purchase landed must track the number.
func TestNumber_AmbiguousBuyErrorTracksANumberThatLanded(t *testing.T) {
	l := newVLC(t, "Number", "n")
	in := buyInputs()
	delete(in, "moHttpUrl")
	l.m.after["POST /number/buy"] = func() {
		l.m.numbers["US:14155550100"] = map[string]any{"country": "US", "msisdn": "14155550100", "type": "mobile-lvn", "features": []string{"SMS"}}
	}
	// The fake applies the purchase but the response is a 503 (the outcome is unknown to the provider).
	l.m.inject["POST /number/buy"] = 503
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in)})
	require.Error(t, err)
	require.Equal(t, "US:14155550100", c.ID, "a purchase that landed must be tracked, not left billing untracked")
	require.NotNil(t, c.PartialState)
	require.Contains(t, c.PartialState.Reasons[0], "purchase landed")
	require.Equal(t, 1, l.m.countCalls("POST /number/buy"), "the purchase must not be replayed")
}

// A definitive refusal (4xx) must not claim a number that was already owned by someone else's program.
func TestNumber_DefinitiveBuyRefusalIsNotTracked(t *testing.T) {
	l := newVLC(t, "Number", "n")
	l.m.inject["POST /number/buy"] = 403
	c, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(buyInputs())})
	require.Error(t, err)
	require.Empty(t, c.ID)
	require.Nil(t, c.PartialState)
	require.Equal(t, 1, l.m.countCalls("GET /account/numbers"), "a 4xx refusal needs no second ownership check")
}

func TestSamePublicKey_IgnoresLineEndingsAndWrapping(t *testing.T) {
	raw := make([]byte, 91)
	for i := range raw {
		raw[i] = byte(i*7 + 3)
	}
	body := base64.StdEncoding.EncodeToString(raw)
	a := "-----BEGIN PUBLIC KEY-----\n" + body[:64] + "\n" + body[64:] + "\n-----END PUBLIC KEY-----"
	crlf := strings.ReplaceAll(a, "\n", "\r\n") + "\r\n"
	rewrapped := "-----BEGIN PUBLIC KEY-----\n" + body[:40] + "\n" + body[40:] + "\n-----END PUBLIC KEY-----\n"
	require.True(t, samePublicKey(a, crlf))
	require.True(t, samePublicKey(a, rewrapped))
	other := "-----BEGIN PUBLIC KEY-----\n" + "B" + body[1:] + "\n-----END PUBLIC KEY-----"
	require.False(t, samePublicKey(a, other))
	require.False(t, samePublicKey("not a pem", a))
}
