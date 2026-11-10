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

func TestAccountSettings_FullLifecycle(t *testing.T) {
	l := newVLC(t, "AccountSettings", "settings")
	in := map[string]any{"moCallBackUrl": "https://example.com/inbound", "drCallBackUrl": "https://example.com/dr"}
	c := l.create(in)
	require.Equal(t, "key12345", c.ID, "the singleton ID is the API key")
	require.Equal(t, "https://example.com/inbound", l.m.acctSettings["mo-callback-url"])
	require.Equal(t, float64(30), c.Properties.Get("maxOutboundRequest").AsNumber())
	require.Equal(t, float64(30), c.Properties.Get("maxInboundRequest").AsNumber(), "numeric strings decode")
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// Refresh sees no drift, then drift made outside Pulumi.
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))
	l.m.mutate(func() { l.m.acctSettings["dr-callback-url"] = "https://elsewhere.example/dr" })
	rd = l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))["drCallBackUrl"])

	// Update restores drift and applies a change.
	in2 := map[string]any{"moCallBackUrl": "https://example.com/inbound2", "drCallBackUrl": "https://example.com/dr"}
	u := l.update(c.ID, rd.Properties, pm(in2), rd.Inputs)
	require.Equal(t, "https://example.com/inbound2", l.m.acctSettings["mo-callback-url"])
	require.Equal(t, "https://example.com/dr", l.m.acctSettings["dr-callback-url"])

	// Dropping a managed URL from the program clears it on the account; the other is untouched.
	in3 := map[string]any{"moCallBackUrl": "https://example.com/inbound2"}
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, u.Properties, pm(in3), pm(in2)))["drCallBackUrl"])
	u = l.update(c.ID, u.Properties, pm(in3), pm(in2))
	require.Equal(t, "", l.m.acctSettings["dr-callback-url"])
	require.Equal(t, "https://example.com/inbound2", l.m.acctSettings["mo-callback-url"])

	// An unmanaged URL changed outside Pulumi is not drift.
	l.m.mutate(func() { l.m.acctSettings["dr-callback-url"] = "https://other.example/dr" })
	rd = l.read(c.ID, u.Properties, pm(in3))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in3), rd.Inputs))
	_, has := rd.Inputs.GetOk("drCallBackUrl")
	require.False(t, has)

	// Import adopts what the account has.
	imp := l.read(c.ID, emptyMap(), emptyMap())
	require.Equal(t, c.ID, imp.ID)
	require.Equal(t, "https://example.com/inbound2", str(imp.Inputs, "moCallBackUrl"))
	require.Equal(t, "https://other.example/dr", str(imp.Inputs, "drCallBackUrl"))
	noChanges(t, l.diff(imp.ID, imp.Properties, imp.Inputs, imp.Inputs))

	// Delete clears only the managed URL.
	l.del(c.ID, u.Properties)
	require.Equal(t, "", l.m.acctSettings["mo-callback-url"])
	require.Equal(t, "https://other.example/dr", l.m.acctSettings["dr-callback-url"])
}

func TestAccountSettings_ReadAndLookupNeverWrite(t *testing.T) {
	l := newVLC(t, "AccountSettings", "settings")
	l.m.mutate(func() {
		l.m.acctSettings = map[string]string{"mo-callback-url": "https://example.com/in", "dr-callback-url": ""}
	})
	got, err := invoke(t, l.s, "vonage:index:getAccountSettings", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/in", got.Get("moCallBackUrl").AsString())
	require.Equal(t, "", got.Get("drCallBackUrl").AsString())
	require.Equal(t, float64(30), got.Get("maxCallsPerSecond").AsNumber())
	l.read("key12345", emptyMap(), emptyMap())
	for _, f := range l.m.acctPosts {
		require.Empty(t, f, "a read must POST no fields")
	}
	require.Len(t, l.m.acctPosts, 2)
}

func TestAccountSettings_CheckAndImportID(t *testing.T) {
	l := newVLC(t, "AccountSettings", "settings")
	for name, in := range map[string]map[string]any{
		"neither set": {},
		"empty":       {"moCallBackUrl": ""},
		"not a url":   {"drCallBackUrl": "ftp:/x"},
	} {
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(in)})
		require.NoError(t, err, name)
		require.NotEmpty(t, chk.Failures, name)
	}
	_, err := l.s.Read(pgp.ReadRequest{Urn: l.urn, ID: "someone-else", Inputs: emptyMap(), Properties: emptyMap()})
	require.ErrorContains(t, err, "invalid AccountSettings ID")
}

func TestAccountSettings_RejectedURLFailsCreateAndPostIsNotRetried(t *testing.T) {
	l := newVLC(t, "AccountSettings", "settings")
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(map[string]any{"moCallBackUrl": "https://reject.example/x"})})
	require.ErrorContains(t, err, "update account settings")
	require.Len(t, l.m.acctPosts, 1)
}
