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
	"testing"

	"github.com/stretchr/testify/require"
)

func envTestStr(s string) *string { return &s }

func envTestDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Environment-only settings must reach Configure even when the engine skips
// CheckConfig, as it does for the provider of an imported resource (#1951).
func TestConfigure_EnvDefaultsWhenCheckConfigSkipped(t *testing.T) {
	t.Setenv("VONAGE_API_KEY", "")
	t.Setenv("VONAGE_API_SECRET", "")
	t.Setenv("VONAGE_APPLICATION_ID", "")
	t.Setenv("VONAGE_PRIVATE_KEY", "")
	t.Setenv("VONAGE_API_BASE_URL", "")
	t.Setenv("VONAGE_REST_BASE_URL", "")
	t.Setenv("VONAGE_API_KEY", "env-APIKey")
	t.Setenv("VONAGE_API_SECRET", "env-APISecret")
	t.Setenv("VONAGE_APPLICATION_ID", "env-ApplicationID")
	t.Setenv("VONAGE_PRIVATE_KEY", "env-PrivateKey")
	t.Setenv("VONAGE_API_BASE_URL", "env-APIBaseURL")
	t.Setenv("VONAGE_REST_BASE_URL", "env-RestBaseURL")
	c := Config{}
	require.NoError(t, c.Configure(context.Background()))
	require.Equal(t, "env-APIKey", envTestDeref(c.APIKey))
	require.Equal(t, "env-APISecret", envTestDeref(c.APISecret))
	require.Equal(t, "env-ApplicationID", envTestDeref(c.ApplicationID))
	require.Equal(t, "env-PrivateKey", envTestDeref(c.PrivateKey))
	require.Equal(t, "env-APIBaseURL", envTestDeref(c.APIBaseURL))
	require.Equal(t, "env-RestBaseURL", envTestDeref(c.RestBaseURL))
}

// A value from stack config wins over the environment, and an unset
// environment leaves the setting unset.
func TestConfigure_EnvDefaultsDoNotOverrideConfig(t *testing.T) {
	t.Setenv("VONAGE_API_KEY", "env-APIKey")
	t.Setenv("VONAGE_API_SECRET", "env-APISecret")
	t.Setenv("VONAGE_APPLICATION_ID", "env-ApplicationID")
	t.Setenv("VONAGE_PRIVATE_KEY", "env-PrivateKey")
	t.Setenv("VONAGE_API_BASE_URL", "env-APIBaseURL")
	t.Setenv("VONAGE_REST_BASE_URL", "env-RestBaseURL")
	c := Config{APIKey: envTestStr("from-config")}
	require.NoError(t, c.Configure(context.Background()))
	require.Equal(t, "from-config", envTestDeref(c.APIKey))
	t.Setenv("VONAGE_API_KEY", "")
	t.Setenv("VONAGE_API_SECRET", "")
	t.Setenv("VONAGE_APPLICATION_ID", "")
	t.Setenv("VONAGE_PRIVATE_KEY", "")
	t.Setenv("VONAGE_API_BASE_URL", "")
	t.Setenv("VONAGE_REST_BASE_URL", "")
	c = Config{}
	require.NoError(t, c.Configure(context.Background()))
	require.Nil(t, c.APIKey)
}
