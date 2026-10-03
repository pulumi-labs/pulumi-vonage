package provider

import (
	"strings"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/stretchr/testify/require"
)

func diffKinds(r pgp.DiffResponse) map[string]pgp.DiffKind {
	out := map[string]pgp.DiffKind{}
	for k, v := range r.DetailedDiff {
		out[k] = v.Kind
	}
	return out
}

func numVoiceInputs() map[string]any {
	return map[string]any{"country": "US", "msisdn": "14155550100", "acquisition": "adopt", "cancelOnDelete": false,
		"voiceCallbackType": "tel", "voiceCallbackValue": "14155550199", "voiceStatusCallback": "https://example.com/vs"}
}

func TestNumber_RemovingVoiceRoutingClearsItOnVonage(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	m.numbers["US:14155550100"] = map[string]any{"country": "US", "msisdn": "14155550100", "type": "mobile-lvn", "features": []any{"VOICE"}}
	urn := vonageURN("Number", "v")
	cr, err := s.Create(pgp.CreateRequest{Urn: urn, Properties: pm(numVoiceInputs())})
	require.NoError(t, err)
	require.Equal(t, "tel", m.numbers["US:14155550100"]["voiceCallbackType"])
	require.Equal(t, "https://example.com/vs", m.numbers["US:14155550100"]["voiceStatusCallback"])

	in2 := numVoiceInputs()
	delete(in2, "voiceCallbackType")
	delete(in2, "voiceCallbackValue")
	delete(in2, "voiceStatusCallback")
	_, err = s.Update(pgp.UpdateRequest{ID: cr.ID, Urn: urn, State: cr.Properties, Inputs: pm(in2), OldInputs: pm(numVoiceInputs())})
	require.NoError(t, err)
	require.Equal(t, "", m.numbers["US:14155550100"]["voiceCallbackType"], "voice routing must be cleared on Vonage")
	require.Equal(t, "", m.numbers["US:14155550100"]["voiceCallbackValue"])
	require.Equal(t, "", m.numbers["US:14155550100"]["voiceStatusCallback"])
}

func TestNumber_ReadSurfacesVoiceDriftAndImportPopulatesState(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	m.numbers["US:14155550100"] = map[string]any{"country": "US", "msisdn": "14155550100", "type": "mobile-lvn", "features": []any{"VOICE"},
		"voiceCallbackType": "tel", "voiceCallbackValue": "14155550199", "voiceStatusCallback": "https://example.com/changed"}
	urn := vonageURN("Number", "v")

	// Refresh of a managed number: voiceStatusCallback drift made outside Pulumi is visible.
	rr, err := s.Read(pgp.ReadRequest{ID: "US:14155550100", Urn: urn, Properties: pm(numVoiceInputs()), Inputs: pm(numVoiceInputs())})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/changed", str(rr.Inputs, "voiceStatusCallback"))
	require.Equal(t, "https://example.com/changed", str(rr.Properties, "voiceStatusCallback"))

	// Import: no inputs at all; routing is populated from Vonage.
	rr, err = s.Read(pgp.ReadRequest{ID: "US:14155550100", Urn: urn, Properties: pm(map[string]any{})})
	require.NoError(t, err)
	require.Equal(t, "tel", str(rr.Inputs, "voiceCallbackType"))
	require.Equal(t, "14155550199", str(rr.Inputs, "voiceCallbackValue"))
	require.Equal(t, "https://example.com/changed", str(rr.Inputs, "voiceStatusCallback"))
	require.Equal(t, "adopt", str(rr.Inputs, "acquisition"))
}

func TestApiSecret_ImportedSecretIsAdoptedNotReplaced(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	m.secrets["key12345"] = map[string]string{"orig": "2025-01-01T00:00:00Z"}
	urn := vonageURN("ApiSecret", "imp")

	rr, err := s.Read(pgp.ReadRequest{ID: "key12345/orig", Urn: urn, Properties: pm(map[string]any{})})
	require.NoError(t, err)
	require.Equal(t, "orig", str(rr.Properties, "secretId"))

	prog := map[string]any{"secret": "Orig12345"}
	d, err := s.Diff(pgp.DiffRequest{ID: rr.ID, Urn: urn, State: rr.Properties, Inputs: pm(prog), OldInputs: rr.Inputs})
	require.NoError(t, err)
	require.True(t, d.HasChanges)
	require.Equal(t, pgp.Update, diffKinds(d)["secret"], "import must adopt in place, never replace")

	before := m.callLog()
	ur, err := s.Update(pgp.UpdateRequest{ID: rr.ID, Urn: urn, State: rr.Properties, Inputs: pm(prog), OldInputs: rr.Inputs})
	require.NoError(t, err)
	require.Equal(t, before, m.callLog(), "adopting the secret must not call Vonage")
	require.Contains(t, m.secrets["key12345"], "orig", "existing secret must not be revoked")

	// After adoption the next diff is clean; a later value change does replace.
	d, err = s.Diff(pgp.DiffRequest{ID: rr.ID, Urn: urn, State: ur.Properties, Inputs: pm(prog), OldInputs: pm(prog)})
	require.NoError(t, err)
	require.False(t, d.HasChanges)
	d, err = s.Diff(pgp.DiffRequest{ID: rr.ID, Urn: urn, State: ur.Properties, Inputs: pm(map[string]any{"secret": "Other12345"}), OldInputs: pm(prog)})
	require.NoError(t, err)
	require.Equal(t, pgp.UpdateReplace, diffKinds(d)["secret"])

	// A different owner replaces.
	d, err = s.Diff(pgp.DiffRequest{ID: rr.ID, Urn: urn, State: ur.Properties, Inputs: pm(map[string]any{"secret": "Orig12345", "accountKey": "sub00001"}), OldInputs: pm(prog)})
	require.NoError(t, err)
	require.Equal(t, pgp.UpdateReplace, diffKinds(d)["accountKey"])
}

func TestApiSecret_DeleteToleratesAlreadyRevoked(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	urn := vonageURN("ApiSecret", "gone")
	state := pm(map[string]any{"secret": "Orig12345", "ownerKey": "key12345", "secretId": "missing", "createdAt": "x"})
	// The mock holds no such secret, so the revoke endpoint answers 404; Delete must treat that as gone.
	require.NoError(t, s.Delete(pgp.DeleteRequest{ID: "key12345/missing", Urn: urn, Properties: state}))
	require.Contains(t, m.callLog(), "DELETE /accounts/key12345/secrets/missing",
		"the revoke request must reach the mock so the 404 branch is actually exercised")
}

func TestApiSecret_ReadGone(t *testing.T) {
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	rr, err := s.Read(pgp.ReadRequest{ID: "key12345/missing", Urn: vonageURN("ApiSecret", "g"), Properties: pm(map[string]any{})})
	require.NoError(t, err)
	require.Empty(t, rr.ID)
}

func TestSubAccount_SecretIsCreateOnly(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	urn := vonageURN("SubAccount", "s")
	in := map[string]any{"name": "team-a", "secret": "Initial123"}
	cr, err := s.Create(pgp.CreateRequest{Urn: urn, Properties: pm(in)})
	require.NoError(t, err)

	in2 := map[string]any{"name": "team-a", "secret": "Changed123"}
	d, err := s.Diff(pgp.DiffRequest{ID: cr.ID, Urn: urn, State: cr.Properties, Inputs: pm(in2), OldInputs: pm(in)})
	require.NoError(t, err)
	require.False(t, d.HasChanges, "a secret-only change must not diff: Vonage cannot apply it")

	in3 := map[string]any{"name": "team-b", "secret": "Changed123"}
	d, err = s.Diff(pgp.DiffRequest{ID: cr.ID, Urn: urn, State: cr.Properties, Inputs: pm(in3), OldInputs: pm(in)})
	require.NoError(t, err)
	require.Equal(t, pgp.Update, diffKinds(d)["name"])
	ur, err := s.Update(pgp.UpdateRequest{ID: cr.ID, Urn: urn, State: cr.Properties, Inputs: pm(in3), OldInputs: pm(in)})
	require.NoError(t, err)
	require.Equal(t, "Initial123", str(ur.Properties, "secret"), "state must keep the secret Vonage actually received")
	require.NotContains(t, m.subs[cr.ID], "secret", "secret must never be sent on PATCH")
}

func TestSubAccount_ReadGone(t *testing.T) {
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	rr, err := s.Read(pgp.ReadRequest{ID: "nope00000", Urn: vonageURN("SubAccount", "g"), Properties: pm(map[string]any{"name": "x"})})
	require.NoError(t, err)
	require.Empty(t, rr.ID)
}

func TestCheckBaseURL(t *testing.T) {
	for _, ok := range []string{"", "https://api.nexmo.com", "http://127.0.0.1:8080", "http://localhost:9", "http://[::1]:1"} {
		require.NoError(t, checkBaseURL(ok), ok)
	}
	for _, bad := range []string{"http://api.nexmo.com", "ftp://x", "api.nexmo.com"} {
		require.Error(t, checkBaseURL(bad), bad)
	}
}

func TestSubAccountCheck_NameLengthCountsCharacters(t *testing.T) {
	// 80 multi-byte characters are valid even though they exceed 80 bytes.
	name := strings.Repeat("é", 80)
	require.Greater(t, len(name), 80)
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	resp, err := s.Check(pgp.CheckRequest{Urn: vonageURN("SubAccount", "n"), Inputs: pm(map[string]any{"name": name})})
	require.NoError(t, err)
	require.Empty(t, resp.Failures)
	resp, err = s.Check(pgp.CheckRequest{Urn: vonageURN("SubAccount", "n"), Inputs: pm(map[string]any{"name": name + "é"})})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Failures)
}
