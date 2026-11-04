package provider

import (
	"fmt"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func invoke(t *testing.T, s integration.Server, tok string, in map[string]any) (property.Map, error) {
	t.Helper()
	r, err := s.Invoke(pgp.InvokeRequest{Token: tokens.Type(tok), Args: pm(in)})
	return r.Return, err
}

func TestVerifyTemplate_FullLifecycle(t *testing.T) {
	l := newVLC(t, "VerifyTemplate", "tpl")
	in := map[string]any{"name": "Brand A", "isDefault": true}
	c := l.create(in)
	require.Equal(t, "tpl-1", c.ID)
	require.Equal(t, "tpl-1", str(c.Properties, "templateId"))
	require.True(t, l.m.templates["tpl-1"]["is_default"].(bool), "isDefault is applied by a PATCH after create")
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// Read, then an unchanged Diff.
	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, "Brand A", str(rd.Properties, "name"))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Update renames and clears the default.
	in2 := map[string]any{"name": "Brand B"}
	d := l.diff(c.ID, c.Properties, pm(in2), pm(in))
	require.Equal(t, pgp.Update, diffKinds(d)["name"])
	require.Equal(t, pgp.Update, diffKinds(d)["isDefault"])
	u := l.update(c.ID, c.Properties, pm(in2), pm(in))
	require.Equal(t, "Brand B", l.m.templates["tpl-1"]["name"])
	require.False(t, l.m.templates["tpl-1"]["is_default"].(bool))
	noChanges(t, l.diff(c.ID, u.Properties, pm(in2), pm(in2)))

	// Made default outside Pulumi: refresh surfaces drift, which the next Diff reports.
	l.m.mutate(func() { l.m.templates["tpl-1"]["is_default"] = true })
	rd = l.read(c.ID, u.Properties, pm(in2))
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, rd.Properties, pm(in2), rd.Inputs))["isDefault"])

	// Import by ID alone then no change once the program is adopted.
	l.m.mutate(func() { l.m.templates["tpl-1"]["is_default"] = false })
	imp := l.read("tpl-1", emptyMap(), emptyMap())
	require.Equal(t, "Brand B", str(imp.Inputs, "name"))
	noChanges(t, l.diff(imp.ID, imp.Properties, pm(in2), imp.Inputs))

	// Deleted outside Pulumi: Read reports gone, Delete is quiet.
	l.m.mutate(func() { delete(l.m.templates, "tpl-1") })
	require.Empty(t, l.read(c.ID, c.Properties, pm(in2)).ID)
	l.del(c.ID, c.Properties)
	l.del(c.ID, c.Properties)
}

func TestVerifyTemplate_Delete(t *testing.T) {
	l := newVLC(t, "VerifyTemplate", "tpl")
	c := l.create(map[string]any{"name": "x"})
	l.del(c.ID, c.Properties)
	require.NotContains(t, l.m.templates, "tpl-1")
}

func TestVerifyTemplate_CreateDefaultFailureKeepsTheTemplate(t *testing.T) {
	l := newVLC(t, "VerifyTemplate", "tpl")
	l.m.inject["PATCH /v2/verify/templates/tpl-1"] = 400
	in := map[string]any{"name": "x", "isDefault": true}
	resp, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in)})
	require.Error(t, err)
	require.Contains(t, l.m.templates, "tpl-1", "the created template must be reported, not orphaned")
	require.Equal(t, "tpl-1", resp.ID)
	// The recorded state must reflect what the server has (not the declared true), so the
	// next Diff reports isDefault as a change and Update retries it.
	require.Equal(t, false, resp.Properties.Get("isDefault").AsBool())
	require.Equal(t, pgp.Update, diffKinds(l.diff(resp.ID, resp.Properties, pm(in), pm(in)))["isDefault"])
}

func TestVerifyTemplate_CheckRejectsEmptyName(t *testing.T) {
	l := newVLC(t, "VerifyTemplate", "tpl")
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(map[string]any{"name": "  "})})
	require.NoError(t, err)
	require.NotEmpty(t, chk.Failures)
}

func TestVerifyTemplateFragment_FullLifecycle(t *testing.T) {
	m, srv := newMock(t)
	tl := &vlc{t: t, s: newServerFor(t, srv.URL), m: m, urn: vonageURN("VerifyTemplate", "tpl")}
	tpl := tl.create(map[string]any{"name": "Brand A"})
	l := &vlc{t: t, s: tl.s, m: m, urn: vonageURN("VerifyTemplateFragment", "frag")}

	in := map[string]any{"templateId": tpl.ID, "channel": "sms", "locale": "en-US", "text": "Your code is ${code}"}
	c := l.create(in)
	require.Equal(t, "tpl-1/frag-1-1", c.ID)
	require.Equal(t, "frag-1-1", str(c.Properties, "templateFragmentId"))
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// Read keeps the declared locale spelling when the server differs only in case.
	l.m.mutate(func() { l.m.fragments["tpl-1"]["frag-1-1"]["locale"] = "en-us" })
	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, "en-US", str(rd.Inputs, "locale"))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Update changes only the text, in place, and refreshes the timestamps.
	in2 := clone(in)
	in2["text"] = "Use ${code} to sign in"
	d := l.diff(c.ID, c.Properties, pm(in2), pm(in))
	require.Equal(t, map[string]pgp.DiffKind{"text": pgp.Update}, diffKinds(d))
	u := l.update(c.ID, c.Properties, pm(in2), pm(in))
	require.Equal(t, "Use ${code} to sign in", l.m.fragments["tpl-1"]["frag-1-1"]["text"])
	require.Equal(t, "2026-02-02T00:00:00Z", str(u.Properties, "dateUpdated"))
	noChanges(t, l.diff(c.ID, u.Properties, pm(in2), pm(in2)))

	// Identity fields replace.
	for _, f := range []string{"templateId", "channel", "locale"} {
		in3 := clone(in2)
		in3[f] = "voice"
		require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, u.Properties, pm(in3), pm(in2)))[f], f)
	}

	// Import by ID alone adopts everything from the server, then no change.
	imp := l.read(c.ID, emptyMap(), emptyMap())
	require.Equal(t, "tpl-1", str(imp.Inputs, "templateId"))
	require.Equal(t, "sms", str(imp.Inputs, "channel"))
	noChanges(t, l.diff(imp.ID, imp.Properties, pm(in2), imp.Inputs))
	for _, bad := range []string{"frag-1-1", "/frag", "tpl-1/", "a/b/c"} {
		_, err := l.s.Read(pgp.ReadRequest{ID: bad, Urn: l.urn, Properties: emptyMap()})
		require.ErrorContains(t, err, "invalid VerifyTemplateFragment ID", bad)
	}

	// Lookup by ID and by channel plus locale, and a clear not-found.
	got, err := invoke(t, l.s, "vonage:index:getVerifyTemplateFragment", map[string]any{"templateId": "tpl-1", "templateFragmentId": "frag-1-1"})
	require.NoError(t, err)
	require.Equal(t, "Use ${code} to sign in", str(got, "text"))
	got, err = invoke(t, l.s, "vonage:index:getVerifyTemplateFragment", map[string]any{"templateId": "tpl-1", "channel": "sms", "locale": "EN-us"})
	require.NoError(t, err)
	require.Equal(t, "frag-1-1", str(got, "templateFragmentId"))
	_, err = invoke(t, l.s, "vonage:index:getVerifyTemplateFragment", map[string]any{"templateId": "tpl-1", "channel": "voice", "locale": "fr"})
	require.ErrorContains(t, err, "was found")
	_, err = invoke(t, l.s, "vonage:index:getVerifyTemplateFragment", map[string]any{"templateId": "tpl-1", "templateFragmentId": "nope"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getVerifyTemplateFragment", map[string]any{"templateId": "tpl-1"})
	require.ErrorContains(t, err, "set templateId and either")

	// Deleted outside Pulumi (fragment, then whole template): Read reports gone, Delete is quiet.
	l.m.mutate(func() { delete(l.m.fragments["tpl-1"], "frag-1-1") })
	require.Empty(t, l.read(c.ID, c.Properties, pm(in)).ID)
	l.del(c.ID, c.Properties)
	c2 := l.create(in)
	l.m.mutate(func() { delete(l.m.templates, "tpl-1"); delete(l.m.fragments, "tpl-1") })
	require.Empty(t, l.read(c2.ID, c2.Properties, pm(in)).ID)
	l.del(c2.ID, c2.Properties)
}

func TestVerifyTemplateFragment_CheckValidates(t *testing.T) {
	l := newVLC(t, "VerifyTemplateFragment", "frag")
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(map[string]any{"templateId": "t", "channel": "email", "locale": "en-us", "text": "x"})})
	require.NoError(t, err)
	require.Len(t, chk.Failures, 1)
	require.Equal(t, "channel", chk.Failures[0].Property)
}

func TestVerifyTemplate_LookupsAndPagination(t *testing.T) {
	l := newVLC(t, "VerifyTemplate", "tpl")
	for _, n := range []string{"a", "b", "b"} {
		l.create(map[string]any{"name": n})
	}
	got, err := invoke(t, l.s, "vonage:index:getVerifyTemplate", map[string]any{"templateId": "tpl-1"})
	require.NoError(t, err)
	require.Equal(t, "a", str(got, "name"))
	got, err = invoke(t, l.s, "vonage:index:getVerifyTemplate", map[string]any{"name": "a"})
	require.NoError(t, err)
	require.Equal(t, "tpl-1", str(got, "templateId"))
	_, err = invoke(t, l.s, "vonage:index:getVerifyTemplate", map[string]any{"name": "b"})
	require.ErrorContains(t, err, "2 Verify templates are named")
	_, err = invoke(t, l.s, "vonage:index:getVerifyTemplate", map[string]any{"name": "zzz"})
	require.ErrorContains(t, err, "was found")
	_, err = invoke(t, l.s, "vonage:index:getVerifyTemplate", map[string]any{"templateId": "missing"})
	require.ErrorContains(t, err, "was found")
	_, err = invoke(t, l.s, "vonage:index:getVerifyTemplate", map[string]any{})
	require.ErrorContains(t, err, "exactly one")
}

func TestClient_VerifyListsWalkEveryPage(t *testing.T) {
	m, srv := newMock(t)
	c := newClient("key12345", "Secret123", srv.URL, srv.URL)
	m.mutate(func() {
		m.templates = map[string]map[string]any{}
		m.fragments = map[string]map[string]map[string]any{"tpl-1": {}}
		for i := 0; i < 250; i++ {
			id := fmt.Sprintf("tpl-%04d", i)
			m.templates[id] = map[string]any{"template_id": id, "name": id, "is_default": false}
		}
		m.templates["tpl-1"] = map[string]any{"template_id": "tpl-1", "name": "x", "is_default": false}
		for i := 0; i < 120; i++ {
			id := fmt.Sprintf("frag-%03d", i)
			m.fragments["tpl-1"][id] = map[string]any{"template_fragment_id": id, "channel": "sms", "locale": "l" + id}
		}
	})
	ts, err := c.ListTemplates(t.Context())
	require.NoError(t, err)
	require.Len(t, ts, 251)
	fs, err := c.ListFragments(t.Context(), "tpl-1")
	require.NoError(t, err)
	require.Len(t, fs, 120)
}
