package provider_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blang/semver"
	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/autoregistry/lib/providers/vonage/provider/pkg/provider"
)

func mustSchema(t *testing.T) schema.PackageSpec {
	t.Helper()
	prov, err := provider.New()
	if err != nil {
		t.Fatalf("provider.New() returned error: %v", err)
	}
	spec, err := pgp.GetSchema(context.Background(), "vonage", "0.1.0", prov)
	if err != nil {
		t.Fatalf("GetSchema failed: %v", err)
	}
	return spec
}

func TestSchema_ReplaceOnChangesForIdentityInputs(t *testing.T) {
	spec := mustSchema(t)
	want := map[string][]string{
		"vonage:index:Number":                 {"country", "msisdn"},
		"vonage:index:VerifyTemplateFragment": {"templateId", "channel", "locale"},
	}
	for tok, props := range want {
		res, ok := spec.Resources[tok]
		if !ok {
			t.Fatalf("%s missing from schema", tok)
		}
		for _, p := range props {
			in, ok := res.InputProperties[p]
			if !ok {
				t.Fatalf("%s missing input %q", tok, p)
			}
			if !in.ReplaceOnChanges {
				t.Errorf("%s.%s must be replaceOnChanges", tok, p)
			}
		}
	}
}

// acquisition only matters at create time. Tagging it replaceOnChanges would plan a
// same-ID replacement whose delete cancels the number.
func TestSchema_NumberAcquisitionIsNotReplaceOnChanges(t *testing.T) {
	spec := mustSchema(t)
	in, ok := spec.Resources["vonage:index:Number"].InputProperties["acquisition"]
	if !ok {
		t.Fatal("Number missing input acquisition")
	}
	if in.ReplaceOnChanges {
		t.Error("Number.acquisition must not be replaceOnChanges (it is create-only)")
	}
}

// Server-assigned-ID resources must not over-tag: renaming an Application or
// SubAccount is an in-place update.
func TestSchema_ServerAssignedIDResourcesNotOverTagged(t *testing.T) {
	spec := mustSchema(t)
	for tok, props := range map[string][]string{
		"vonage:index:Application": {"name", "voice", "messages"},
		"vonage:index:SubAccount":  {"name", "suspended"},
		"vonage:index:Number":      {"appId", "moHttpUrl", "cancelOnDelete"},
	} {
		res := spec.Resources[tok]
		for _, p := range props {
			if res.InputProperties[p].ReplaceOnChanges {
				t.Errorf("%s.%s must not be replaceOnChanges", tok, p)
			}
		}
	}
}

func TestSchema_SecretsAreMarked(t *testing.T) {
	spec := mustSchema(t)
	for tok, p := range map[string]string{
		"vonage:index:Application": "privateKey",
		"vonage:index:SubAccount":  "generatedSecret",
		"vonage:index:ApiSecret":   "secret",
	} {
		if !spec.Resources[tok].Properties[p].Secret {
			t.Errorf("%s.%s must be secret", tok, p)
		}
	}
	for _, k := range []string{"apiSecret", "privateKey"} {
		if !spec.Config.Variables[k].Secret {
			t.Errorf("config %s must be secret", k)
		}
	}
}

func TestSchema_Metadata(t *testing.T) {
	spec := mustSchema(t)
	if spec.Repository != "https://github.com/pulumi/autoregistry" {
		t.Errorf("repository = %q", spec.Repository)
	}
	if spec.Homepage == "" || spec.DisplayName != "Vonage" {
		t.Errorf("homepage/displayName wrong: %q %q", spec.Homepage, spec.DisplayName)
	}
	if len(spec.Keywords) == 0 || spec.Keywords[0] != "pulumi" {
		t.Errorf("keywords must start with pulumi: %v", spec.Keywords)
	}
	if _, ok := spec.Functions["vonage:index:getApplication"]; !ok {
		t.Error("getApplication missing")
	}
	if _, ok := spec.Functions["vonage:index:getNumber"]; !ok {
		t.Error("getNumber missing")
	}
}

// ApiSecret replacement is decided by its custom Diff (an imported secret is
// adopted in place instead of being replaced). A schema-level replaceOnChanges
// would make the engine force a replace on import, minting a new secret and
// revoking the old one, so these inputs must not carry the flag.
func TestSchema_ApiSecretInputsAreNotReplaceOnChanges(t *testing.T) {
	spec := mustSchema(t)
	res, ok := spec.Resources["vonage:index:ApiSecret"]
	if !ok {
		t.Fatal("vonage:index:ApiSecret missing from schema")
	}
	for _, p := range []string{"accountKey", "secret"} {
		in, ok := res.InputProperties[p]
		if !ok {
			t.Fatalf("ApiSecret missing input %q", p)
		}
		if in.ReplaceOnChanges {
			t.Errorf("ApiSecret.%s must not be schema replaceOnChanges; Diff decides replacement", p)
		}
	}
}

// ---------------------------------------------------------------------------
// CheckConfig: environment variable fallbacks
// ---------------------------------------------------------------------------

// envConfigCase describes one provider config key and the environment
// variables (in precedence order) that back it.
type envConfigCase struct {
	key     string
	envVars []string
	secret  bool
}

var envConfigCases = []envConfigCase{
	{key: "apiKey", envVars: []string{"VONAGE_API_KEY"}, secret: false},
	{key: "apiSecret", envVars: []string{"VONAGE_API_SECRET"}, secret: true},
	{key: "applicationId", envVars: []string{"VONAGE_APPLICATION_ID"}, secret: false},
	{key: "privateKey", envVars: []string{"VONAGE_PRIVATE_KEY"}, secret: true},
	{key: "apiBaseUrl", envVars: []string{"VONAGE_API_BASE_URL"}, secret: false},
	{key: "restBaseUrl", envVars: []string{"VONAGE_REST_BASE_URL"}, secret: false},
}

// allEnvVars lists every environment variable the provider reads.
func allEnvVars() []string {
	return []string{"VONAGE_API_BASE_URL", "VONAGE_API_KEY", "VONAGE_API_SECRET", "VONAGE_APPLICATION_ID", "VONAGE_PRIVATE_KEY", "VONAGE_REST_BASE_URL"}
}

// newEnvTestServer builds an integration server for the provider and clears
// every provider environment variable so each case starts hermetic.
func newEnvTestServer(t *testing.T) integration.Server {
	t.Helper()
	for _, v := range allEnvVars() {
		t.Setenv(v, "")
	}
	p, err := provider.New()
	require.NoError(t, err)
	s, err := integration.NewServer(t.Context(), "vonage", semver.MustParse("0.0.1"), integration.WithProvider(p))
	require.NoError(t, err)
	return s
}

// checkConfigValue runs CheckConfig with cfg and returns the plain string value of key.
func checkConfigValue(t *testing.T, s integration.Server, cfg map[string]property.Value, key string) string {
	t.Helper()
	resp, err := s.CheckConfig(pgp.CheckRequest{
		Urn:    resource.NewURN("stack", "proj", "", "pulumi:providers:vonage", "default"),
		Inputs: property.NewMap(cfg),
	})
	require.NoError(t, err)
	require.Empty(t, resp.Failures)
	got, ok := resp.Inputs.GetOk(key)
	require.True(t, ok, "%s should be populated", key)
	if got.Secret() {
		got = got.WithSecret(false)
	}
	return got.AsString()
}

// TestCheckConfig_EnvFallbacks verifies that unset provider config is filled
// from the environment, that explicit config wins, and that the first
// non-empty environment variable wins when a key has several.
func TestCheckConfig_EnvFallbacks(t *testing.T) {
	type tc struct {
		name   string
		key    string
		env    map[string]string
		config map[string]property.Value
		want   string
	}
	var tests []tc
	for _, c := range envConfigCases {
		for _, v := range c.envVars {
			tests = append(tests, tc{name: c.key + " from " + v, key: c.key, env: map[string]string{v: "from-" + v}, want: "from-" + v})
		}
		if len(c.envVars) > 1 {
			env := map[string]string{}
			for _, v := range c.envVars {
				env[v] = "from-" + v
			}
			tests = append(tests, tc{name: c.key + " first env var wins", key: c.key, env: env, want: "from-" + c.envVars[0]})
		}
		tests = append(tests, tc{
			name:   c.key + " explicit config wins over env",
			key:    c.key,
			env:    map[string]string{c.envVars[0]: "from-env"},
			config: map[string]property.Value{c.key: property.New("from-config")},
			want:   "from-config",
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newEnvTestServer(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			require.Equal(t, tt.want, checkConfigValue(t, s, tt.config, tt.key))
		})
	}
}

// TestCheckConfig_EnvSecrets verifies that env-sourced values for secret
// config keys are returned marked secret.
func TestCheckConfig_EnvSecrets(t *testing.T) {
	for _, c := range envConfigCases {
		if !c.secret {
			continue
		}
		t.Run(c.key, func(t *testing.T) {
			s := newEnvTestServer(t)
			t.Setenv(c.envVars[0], "from-env")
			resp, err := s.CheckConfig(pgp.CheckRequest{
				Urn:    resource.NewURN("stack", "proj", "", "pulumi:providers:vonage", "default"),
				Inputs: property.NewMap(nil),
			})
			require.NoError(t, err)
			got, ok := resp.Inputs.GetOk(c.key)
			require.True(t, ok)
			require.True(t, got.Secret(), "%s sourced from env should be secret", c.key)
		})
	}
}

// TestSchema_ConfigEnvironmentDefaults verifies the generated schema advertises
// the environment variable fallbacks for every config key.
func TestSchema_ConfigEnvironmentDefaults(t *testing.T) {
	s := newEnvTestServer(t)
	resp, err := s.GetSchema(pgp.GetSchemaRequest{})
	require.NoError(t, err)

	var spec struct {
		Config struct {
			Variables map[string]struct {
				DefaultInfo struct {
					Environment []string `json:"environment"`
				} `json:"defaultInfo"`
			} `json:"variables"`
		} `json:"config"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Schema), &spec))
	for _, c := range envConfigCases {
		v, ok := spec.Config.Variables[c.key]
		require.True(t, ok, "config.variables.%s missing from schema", c.key)
		require.Equal(t, c.envVars, v.DefaultInfo.Environment, "config.variables.%s.defaultInfo.environment", c.key)
	}
}
