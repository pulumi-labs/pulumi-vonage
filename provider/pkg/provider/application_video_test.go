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

func videoInputs() map[string]any {
	in := appInputs()
	in["video"] = map[string]any{
		"webhooks": map[string]any{
			"archiveStatus": map[string]any{"address": "https://example.com/archive", "active": true},
			"streamCreated": map[string]any{"address": "https://example.com/stream"},
		},
		"storage": map[string]any{"serverSideEncryption": true, "cloudStorage": false},
	}
	in["networkApis"] = map[string]any{"networkApplicationId": "na-1", "redirectUri": "https://example.com/cb"}
	return in
}

func TestApplication_VideoAndNetworkApisRoundTrip(t *testing.T) {
	l := newVLC(t, "Application", "app")
	in := videoInputs()
	c := l.create(in)

	caps := l.m.app(c.ID)["capabilities"].(map[string]any)
	video := caps["video"].(map[string]any)
	hooks := video["webhooks"].(map[string]any)
	require.Equal(t, "https://example.com/archive", hooks["archive_status"].(map[string]any)["address"])
	require.Equal(t, true, hooks["archive_status"].(map[string]any)["active"])
	require.Equal(t, "https://example.com/stream", hooks["stream_created"].(map[string]any)["address"])
	require.Equal(t, true, video["storage"].(map[string]any)["server_side_encryption"])
	require.Equal(t, false, video["storage"].(map[string]any)["cloud_storage"])
	na := caps["network_apis"].(map[string]any)
	require.Equal(t, "na-1", na["network_application_id"])
	require.Equal(t, "https://example.com/cb", na["redirect_uri"])

	// Refresh shows no drift.
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// A change made outside Pulumi shows up as drift.
	l.m.mutate(func() {
		h := l.m.apps[c.ID]["capabilities"].(map[string]any)["video"].(map[string]any)["webhooks"].(map[string]any)
		h["archive_status"] = map[string]any{"address": "https://example.com/other", "active": true}
	})
	rd = l.read(c.ID, c.Properties, pm(in))
	d := l.diff(c.ID, rd.Properties, pm(in), rd.Inputs)
	require.True(t, d.HasChanges)
}

func TestApplication_VideoRemovedFromProgramDisablesIt(t *testing.T) {
	l := newVLC(t, "Application", "app")
	in := videoInputs()
	c := l.create(in)

	in2 := clone(in)
	delete(in2, "video")
	l.update(c.ID, c.Properties, pm(in2), pm(in))
	caps := l.m.app(c.ID)["capabilities"].(map[string]any)
	require.NotContains(t, caps, "video", "a block removed from the program is removed on the server")
	require.Contains(t, caps, "network_apis", "the block still declared stays")
}

func TestApplication_UndeclaredVideoIsPreservedAndNotDrift(t *testing.T) {
	l := newVLC(t, "Application", "app")
	in := appInputs()
	c := l.create(in)
	l.m.mutate(func() {
		l.m.apps[c.ID]["capabilities"].(map[string]any)["video"] = map[string]any{
			"webhooks": map[string]any{"session_created": map[string]any{"address": "https://example.com/s", "active": true}}}
	})
	rd := l.read(c.ID, c.Properties, pm(in))
	_, has := rd.Inputs.GetOk("video")
	require.False(t, has)
	in2 := clone(in)
	in2["name"] = "renamed"
	l.update(c.ID, rd.Properties, pm(in2), rd.Inputs)
	require.Contains(t, l.m.app(c.ID)["capabilities"].(map[string]any), "video")
}

func TestApplication_CheckRejectsEmptyVideoAndNetworkAddresses(t *testing.T) {
	_, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	cases := map[string]map[string]any{
		"empty video address": {"name": "x", "video": map[string]any{"webhooks": map[string]any{
			"renderStatus": map[string]any{"address": " "}}}},
		"empty network id": {"name": "x", "networkApis": map[string]any{"networkApplicationId": ""}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			resp, err := s.Check(pgp.CheckRequest{Urn: vonageURN("Application", "x"), Inputs: pm(in)})
			require.NoError(t, err)
			require.NotEmpty(t, resp.Failures)
		})
	}
}
