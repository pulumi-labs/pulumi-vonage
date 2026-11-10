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
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func userInputs() map[string]any {
	return map[string]any{
		"name":        "alice",
		"displayName": "Alice",
		"imageUrl":    "https://example.com/alice.png",
		"customData":  map[string]any{"tier": "gold", "limits": map[string]any{"calls": 5}},
		"channels": map[string]any{
			"pstn":      []any{map[string]any{"number": 14155550100}},
			"sip":       []any{map[string]any{"uri": "sip:alice@example.com", "username": "alice", "password": "hunter2"}},
			"websocket": []any{map[string]any{"uri": "wss://example.com/ws", "contentType": "audio/l16;rate=16000", "headers": map[string]any{"X-Token": "t"}}},
			"sms":       []any{map[string]any{"number": "14155550100"}},
			"messenger": []any{map[string]any{"id": "12345"}},
		},
	}
}

func TestUser_FullLifecycle(t *testing.T) {
	l, m := newUserLC(t, "alice")
	in := userInputs()
	c := l.create(in)
	require.Equal(t, "USR-00000001", c.ID)
	require.Equal(t, "alice", str(c.Properties, "name"))

	// The wire body uses the API's field names, including the hyphenated content-type.
	require.Equal(t, "Alice", m.lastCreate["display_name"])
	require.Equal(t, "https://example.com/alice.png", m.lastCreate["image_url"])
	require.Equal(t, "gold", m.lastCreate["properties"].(map[string]any)["custom_data"].(map[string]any)["tier"])
	ch := m.lastCreate["channels"].(map[string]any)
	require.EqualValues(t, 14155550100, ch["pstn"].([]any)[0].(map[string]any)["number"])
	require.Equal(t, "audio/l16;rate=16000", ch["websocket"].([]any)[0].(map[string]any)["content-type"])
	require.Equal(t, "12345", ch["messenger"].([]any)[0].(map[string]any)["id"])
	require.Greater(t, m.jwtSeen, 0)

	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// Refresh reproduces the inputs, including the SIP password the API never returns.
	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, c.ID, rd.ID)
	sip := rd.Inputs.Get("channels").AsMap().Get("sip").AsArray().Get(0).AsMap()
	require.Equal(t, "hunter2", str(sip, "password"))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Update: change a scalar, drop one, shrink the channel lists.
	in2 := clone(in)
	in2["displayName"] = "Alice B"
	delete(in2, "imageUrl")
	delete(in2["channels"].(map[string]any), "messenger")
	in2["channels"].(map[string]any)["sms"] = []any{map[string]any{"number": "14155550101"}}
	in2["customData"] = map[string]any{"tier": "silver"}
	d := l.diff(c.ID, rd.Properties, pm(in2), pm(in))
	kinds := diffKinds(d)
	require.Equal(t, pgp.Update, kinds["displayName"])
	require.Equal(t, pgp.Update, kinds["imageUrl"])
	require.Equal(t, pgp.Update, kinds["customData"])
	require.Equal(t, pgp.Update, kinds["channels"])
	require.NotContains(t, kinds, "name")
	up := l.update(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, "alice", str(up.Properties, "name"))
	require.Equal(t, "Alice B", m.lastPatch["display_name"])
	v, present := m.lastPatch["image_url"]
	require.True(t, present)
	require.Nil(t, v, "a removed image is cleared with null")
	require.Equal(t, []any{}, m.lastPatch["channels"].(map[string]any)["messenger"], "a removed channel type is cleared with an empty list")
	require.Equal(t, "Alice B", m.users[c.ID]["display_name"])
	require.NotContains(t, m.users[c.ID], "image_url")
	require.NotContains(t, m.users[c.ID]["channels"], "messenger")
	require.Contains(t, m.users[c.ID]["channels"], "pstn", "untouched channel types survive")
	noChanges(t, l.diff(c.ID, up.Properties, pm(in2), pm(in2)))

	// Changing an explicit name replaces the user.
	in3 := clone(in2)
	in3["name"] = "alice2"
	require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, up.Properties, pm(in3), pm(in2)))["name"])

	l.del(c.ID, up.Properties)
	require.NotContains(t, m.users, c.ID)
	l.del(c.ID, up.Properties) // already gone: quiet
}

func TestUser_GeneratedNameIsNotADiffAndMinimalUserWorks(t *testing.T) {
	l, m := newUserLC(t, "anon")
	in := map[string]any{}
	c := l.create(in)
	require.Equal(t, map[string]any{}, m.lastCreate, "an empty user sends an empty body")
	require.Equal(t, "NAM-00000001", str(c.Properties, "name"), "the generated name lands in state")
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))
	// An all-empty program is indistinguishable from an import, so a refresh adopts the name
	// into the inputs; that is harmless because an unset name never differs.
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// With any other input the generated name stays out of the inputs.
	in2 := map[string]any{"displayName": "Anon"}
	c2 := l.create(in2)
	rd2 := l.read(c2.ID, c2.Properties, pm(in2))
	require.Equal(t, "NAM-00000002", str(rd2.Properties, "name"))
	_, hasName := rd2.Inputs.GetOk("name")
	require.False(t, hasName, "a generated name is not adopted into the inputs")
	noChanges(t, l.diff(c2.ID, rd2.Properties, pm(in2), rd2.Inputs))
}

func TestUser_ImportAndDeletedOutsidePulumi(t *testing.T) {
	l, m := newUserLC(t, "imp")
	m.mutate(func() {
		m.users["USR-adopt"] = map[string]any{"id": "USR-adopt", "name": "bob", "display_name": "Bob",
			"properties": map[string]any{"custom_data": map[string]any{"k": "v"}},
			"channels":   map[string]any{"vbc": []any{map[string]any{"extension": "1234"}}, "viber": []any{map[string]any{"number": "4915551234"}}}}
	})
	rd := l.read("USR-adopt", emptyMap(), emptyMap())
	require.Equal(t, "USR-adopt", rd.ID)
	require.Equal(t, "bob", str(rd.Inputs, "name"))
	require.Equal(t, "Bob", str(rd.Inputs, "displayName"))
	require.Equal(t, "v", str(rd.Inputs.Get("customData").AsMap(), "k"))
	require.Equal(t, "1234", str(rd.Inputs.Get("channels").AsMap().Get("vbc").AsArray().Get(0).AsMap(), "extension"))
	require.Equal(t, "4915551234", str(rd.Inputs.Get("channels").AsMap().Get("viber").AsArray().Get(0).AsMap(), "number"))
	noChanges(t, l.diff(rd.ID, rd.Properties, rd.Inputs, rd.Inputs))

	m.mutate(func() { delete(m.users, "USR-adopt") })
	gone := l.read("USR-adopt", rd.Properties, rd.Inputs)
	require.Empty(t, gone.ID, "a user deleted outside Pulumi reads as gone")
	l.requireImportFails("USR-adopt")
}

func TestUser_CheckRejectsBadChannels(t *testing.T) {
	l, _ := newUserLC(t, "chk")
	bad := map[string]any{"name": " ", "channels": map[string]any{
		"pstn":      []any{map[string]any{"number": 0}},
		"sip":       []any{map[string]any{"uri": "alice@example.com"}},
		"websocket": []any{map[string]any{"uri": "https://example.com"}},
	}}
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(bad)})
	require.NoError(t, err)
	props := map[string]bool{}
	for _, f := range chk.Failures {
		props[f.Property] = true
	}
	require.Equal(t, map[string]bool{"name": true, "channels.pstn[0].number": true, "channels.sip[0].uri": true, "channels.websocket[0].uri": true}, props)
	good := userInputs()
	l.check(good)
}

func TestUser_PreviewMakesNoCallsAndNoID(t *testing.T) {
	l, m := newUserLC(t, "dry")
	r, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(userInputs()), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, r.ID)
	require.Empty(t, m.calls)
}

func TestUser_RetriesTransientReadFailureWithFreshJWT(t *testing.T) {
	l, m := newUserLC(t, "retry")
	c := l.create(map[string]any{"name": "r"})
	m.mutate(func() { m.flaky = 2 })
	before := m.jwtSeen
	rd := l.read(c.ID, c.Properties, pm(map[string]any{"name": "r"}))
	require.Equal(t, c.ID, rd.ID)
	require.Equal(t, 3, m.jwtSeen-before, "each attempt is signed afresh and accepted")
}

func TestUser_CreateConflictIsReported(t *testing.T) {
	l, _ := newUserLC(t, "dup")
	l.create(map[string]any{"name": "same"})
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(map[string]any{"name": "same"})})
	require.ErrorContains(t, err, "409")
}

func TestUser_MissingCredentialsNameTheMissingPiece(t *testing.T) {
	k := testKey(t)
	cases := map[string]struct {
		cfg  map[string]property.Value
		want string
	}{
		"only account creds": {map[string]property.Value{"apiKey": property.New("key12345"), "apiSecret": property.New("Secret123")}, "applicationId is required"},
		"no key":             {map[string]property.Value{"applicationId": property.New("app")}, "VONAGE_PRIVATE_KEY"},
		"bad key":            {map[string]property.Value{"applicationId": property.New("app"), "privateKey": property.New("NOT-A-KEY-SECRET")}, "not PEM encoded"},
		"plain http":         {map[string]property.Value{"applicationId": property.New("app"), "privateKey": property.New(pkcs8PEM(t, k)), "apiBaseUrl": property.New("http://example.com")}, "must use https"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := newJWTServer(t, tc.cfg)
			urn := vonageURN("User", "x")
			chk, err := s.Check(pgp.CheckRequest{Urn: urn, Inputs: pm(map[string]any{})})
			require.NoError(t, err)
			_, err = s.Create(pgp.CreateRequest{Urn: urn, Properties: chk.Inputs})
			require.ErrorContains(t, err, tc.want)
			require.NotContains(t, err.Error(), "NOT-A-KEY-SECRET")
			_, err = invoke(t, s, "vonage:index:getUsers", map[string]any{})
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestGetUser_ByIDByNameAndErrors(t *testing.T) {
	l, m := newUserLC(t, "lk")
	in := userInputs()
	c := l.create(in)
	l.create(map[string]any{"name": "other"})

	byID, err := invoke(t, l.s, "vonage:index:getUser", map[string]any{"userId": c.ID})
	require.NoError(t, err)
	require.Equal(t, c.ID, str(byID, "userId"))
	require.Equal(t, "alice", str(byID, "name"))
	require.Equal(t, "Alice", str(byID, "displayName"))
	require.Equal(t, "gold", str(byID.Get("customData").AsMap(), "tier"))
	require.Equal(t, "sip:alice@example.com", str(byID.Get("channels").AsMap().Get("sip").AsArray().Get(0).AsMap(), "uri"))

	byName, err := invoke(t, l.s, "vonage:index:getUser", map[string]any{"name": "alice"})
	require.NoError(t, err)
	require.Equal(t, c.ID, str(byName, "userId"))

	_, err = invoke(t, l.s, "vonage:index:getUser", map[string]any{"name": "nobody"})
	require.ErrorContains(t, err, "no user named")
	_, err = invoke(t, l.s, "vonage:index:getUser", map[string]any{"userId": "USR-nope"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getUser", map[string]any{})
	require.ErrorContains(t, err, "exactly one")
	_, err = invoke(t, l.s, "vonage:index:getUser", map[string]any{"userId": c.ID, "name": "alice"})
	require.ErrorContains(t, err, "exactly one")
	_ = m
}

func TestGetUsers_FollowsEveryPageAndFiltersByName(t *testing.T) {
	l, m := newUserLC(t, "ls")
	m.mutate(func() { m.pageSz = 2 })
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		l.create(map[string]any{"name": n, "displayName": "D-" + n})
	}
	all, err := invoke(t, l.s, "vonage:index:getUsers", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 5, all.Get("users").AsArray().Len())
	require.Equal(t, 3, m.callCount("GET /v1/users"), "5 users at 2 per page take 3 requests")
	first := all.Get("users").AsArray().Get(0).AsMap()
	require.Equal(t, "a", str(first, "name"))
	require.Equal(t, "D-a", str(first, "displayName"))

	one, err := invoke(t, l.s, "vonage:index:getUsers", map[string]any{"name": "c"})
	require.NoError(t, err)
	require.Equal(t, 1, one.Get("users").AsArray().Len())
	none, err := invoke(t, l.s, "vonage:index:getUsers", map[string]any{"name": "zzz"})
	require.NoError(t, err)
	require.Equal(t, 0, none.Get("users").AsArray().Len())

	// A cursor that never advances is an error, not a truncated list.
	m.mutate(func() { m.brokenCursor = true })
	_, err = invoke(t, l.s, "vonage:index:getUsers", map[string]any{})
	require.ErrorContains(t, err, "does not advance the cursor")
}
