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

func numInputs(acq string, cancel bool) map[string]any {
	return map[string]any{"country": "US", "msisdn": "14155550100", "acquisition": acq, "cancelOnDelete": cancel,
		"moHttpUrl": "https://example.com/sms"}
}

func TestNumber_BuyUpdateCancel(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	urn := vonageURN("Number", "n")

	cr, err := s.Create(pgp.CreateRequest{Urn: urn, Properties: pm(numInputs("buy", true)), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, m.callLog(), "preview must never buy")
	_ = cr

	cr, err = s.Create(pgp.CreateRequest{Urn: urn, Properties: pm(numInputs("buy", true))})
	require.NoError(t, err)
	require.Equal(t, "US:14155550100", cr.ID)
	require.Equal(t, "https://example.com/sms", m.numbers["US:14155550100"]["moHttpUrl"])
	require.Contains(t, m.callLog(), "POST /number/buy")

	// Buying a number that is already owned fails (billing safety), no second purchase.
	_, err = s.Create(pgp.CreateRequest{Urn: vonageURN("Number", "dup"), Properties: pm(numInputs("buy", true))})
	require.Error(t, err)

	in2 := numInputs("buy", true)
	in2["moHttpUrl"] = "https://example.com/sms2"
	ur, err := s.Update(pgp.UpdateRequest{ID: cr.ID, Urn: urn, State: cr.Properties, Inputs: pm(in2), OldInputs: pm(numInputs("buy", true))})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/sms2", str(ur.Properties, "moHttpUrl"))

	require.NoError(t, s.Delete(pgp.DeleteRequest{ID: cr.ID, Urn: urn, Properties: ur.Properties}))
	require.Contains(t, m.callLog(), "POST /number/cancel")
	require.NotContains(t, m.numbers, "US:14155550100")
}

func TestNumber_DeleteWithoutCancelKeepsNumber(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	m.numbers["US:14155550100"] = map[string]any{"country": "US", "msisdn": "14155550100", "type": "mobile-lvn", "features": []string{"SMS"}}
	urn := vonageURN("Number", "adopt")
	cr, err := s.Create(pgp.CreateRequest{Urn: urn, Properties: pm(numInputs("adopt", false))})
	require.NoError(t, err)
	require.NotContains(t, m.callLog(), "POST /number/buy", "adopt must not purchase")
	require.NoError(t, s.Delete(pgp.DeleteRequest{ID: cr.ID, Urn: urn, Properties: cr.Properties}))
	require.NotContains(t, m.callLog(), "POST /number/cancel")
	require.Contains(t, m.numbers, "US:14155550100")
}

func TestNumber_AdoptMissingFails(t *testing.T) {
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	_, err := s.Create(pgp.CreateRequest{Urn: vonageURN("Number", "x"), Properties: pm(numInputs("adopt", false))})
	require.Error(t, err)
}

func TestNumber_CheckValidation(t *testing.T) {
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	bad := map[string]map[string]any{
		"bad acquisition": {"country": "US", "msisdn": "1415", "acquisition": "steal", "cancelOnDelete": true},
		"plus msisdn":     {"country": "US", "msisdn": "+14155550100", "acquisition": "buy", "cancelOnDelete": true},
		"lower country":   {"country": "us", "msisdn": "14155550100", "acquisition": "buy", "cancelOnDelete": true},
		"app and mo url":  {"country": "US", "msisdn": "14155550100", "acquisition": "buy", "cancelOnDelete": true, "appId": "a", "moHttpUrl": "https://e/x"},
	}
	for name, in := range bad {
		t.Run(name, func(t *testing.T) {
			r, err := s.Check(pgp.CheckRequest{Urn: vonageURN("Number", "x"), Inputs: pm(in)})
			require.NoError(t, err)
			require.NotEmpty(t, r.Failures)
		})
	}
	r, err := s.Check(pgp.CheckRequest{Urn: vonageURN("Number", "x"), Inputs: pm(numInputs("buy", false))})
	require.NoError(t, err)
	require.Empty(t, r.Failures)
}

func TestNumber_ReadGoneAndDataSource(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	m.numbers["US:14155550100"] = map[string]any{"country": "US", "msisdn": "14155550100", "type": "mobile-lvn", "features": []any{"SMS"}, "app_id": "app-9"}
	r, err := s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getNumber", Args: pm(map[string]any{"country": "US", "msisdn": "14155550100"})})
	require.NoError(t, err)
	require.Equal(t, "app-9", str(r.Return, "appId"))
	_, err = s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getNumber", Args: pm(map[string]any{"country": "US", "msisdn": "19999999999"})})
	require.Error(t, err)
	rr, err := s.Read(pgp.ReadRequest{ID: "US:19999999999", Urn: vonageURN("Number", "g"),
		Properties: pm(map[string]any{"country": "US", "msisdn": "19999999999", "acquisition": "buy", "cancelOnDelete": true})})
	require.NoError(t, err)
	require.Empty(t, rr.ID)
}

func TestGetApplication_ByIDAndName(t *testing.T) {
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	_, err := s.Create(pgp.CreateRequest{Urn: vonageURN("Application", "a"), Properties: pm(appInputs())})
	require.NoError(t, err)
	r, err := s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getApplication", Args: pm(map[string]any{"name": "demo"})})
	require.NoError(t, err)
	require.Equal(t, "app-1", str(r.Return, "applicationId"))
	caps, _ := r.Return.GetOk("capabilities")
	require.Equal(t, 2, caps.AsArray().Len())
	r, err = s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getApplication", Args: pm(map[string]any{"applicationId": "app-1"})})
	require.NoError(t, err)
	require.Equal(t, "demo", str(r.Return, "name"))
	_, err = s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getApplication", Args: pm(map[string]any{})})
	require.Error(t, err)
	_, err = s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getApplication", Args: pm(map[string]any{"applicationId": "a", "name": "b"})})
	require.Error(t, err)
}

func TestSubAccount_Lifecycle(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	urn := vonageURN("SubAccount", "sub")
	in := map[string]any{"name": "team-a", "suspendOnDelete": true}
	cr, err := s.Create(pgp.CreateRequest{Urn: urn, Properties: pm(in)})
	require.NoError(t, err)
	require.Equal(t, "sub00001", cr.ID)
	require.Equal(t, "GenSecret1", str(cr.Properties, "generatedSecret"))

	in2 := map[string]any{"name": "team-a2", "suspendOnDelete": true}
	ur, err := s.Update(pgp.UpdateRequest{ID: cr.ID, Urn: urn, State: cr.Properties, Inputs: pm(in2), OldInputs: pm(in)})
	require.NoError(t, err)
	require.Equal(t, "team-a2", str(ur.Properties, "name"))
	require.Equal(t, "GenSecret1", str(ur.Properties, "generatedSecret"), "create-only secret preserved")

	require.NoError(t, s.Delete(pgp.DeleteRequest{ID: cr.ID, Urn: urn, Properties: ur.Properties}))
	require.Equal(t, true, m.subs["sub00001"]["suspended"], "delete suspends, never deletes")
}

func TestSubAccount_DeleteWithoutSuspendIsStateOnly(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	urn := vonageURN("SubAccount", "sub")
	cr, err := s.Create(pgp.CreateRequest{Urn: urn, Properties: pm(map[string]any{"name": "keep"})})
	require.NoError(t, err)
	require.NoError(t, s.Delete(pgp.DeleteRequest{ID: cr.ID, Urn: urn, Properties: cr.Properties}))
	require.Equal(t, false, m.subs["sub00001"]["suspended"])
}

func TestSubAccount_CheckRejectsBadSecretAndName(t *testing.T) {
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	for name, in := range map[string]map[string]any{
		"weak secret": {"name": "a", "secret": "short"},
		"empty name":  {"name": ""},
	} {
		t.Run(name, func(t *testing.T) {
			r, err := s.Check(pgp.CheckRequest{Urn: vonageURN("SubAccount", "x"), Inputs: pm(in)})
			require.NoError(t, err)
			require.NotEmpty(t, r.Failures)
		})
	}
}

func TestApiSecret_RotationAndLimits(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	// Account already has its original secret.
	m.secrets["key12345"] = map[string]string{"orig": "2025-01-01T00:00:00Z"}
	urn := vonageURN("ApiSecret", "new")
	cr, err := s.Create(pgp.CreateRequest{Urn: urn, Properties: pm(map[string]any{"secret": "NewSecret9"})})
	require.NoError(t, err)
	require.Equal(t, "sec-1", str(cr.Properties, "secretId"))
	require.Equal(t, "key12345", str(cr.Properties, "ownerKey"))

	// Third secret is refused by the API and surfaces as an error.
	_, err = s.Create(pgp.CreateRequest{Urn: vonageURN("ApiSecret", "third"), Properties: pm(map[string]any{"secret": "Third1234"})})
	require.Error(t, err)

	// Revoke the original, then the new one cannot be removed (last secret).
	require.NoError(t, m.revokeForTest("key12345", "orig"))
	require.Error(t, s.Delete(pgp.DeleteRequest{ID: cr.ID, Urn: urn, Properties: cr.Properties}))

	r, err := s.Check(pgp.CheckRequest{Urn: urn, Inputs: pm(map[string]any{"secret": "weak"})})
	require.NoError(t, err)
	require.NotEmpty(t, r.Failures)
}

func (m *mockVonage) revokeForTest(acct, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.secrets[acct], id)
	return nil
}

func TestAuthFailureSurfaces(t *testing.T) {
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	_ = s
	c := newClient("key12345", "wrong", srv.URL, srv.URL)
	_, err := c.ListApplications(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}
