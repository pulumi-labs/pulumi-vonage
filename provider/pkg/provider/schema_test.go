package provider_test

import (
	"context"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

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
		"vonage:index:Number": {"country", "msisdn", "acquisition"},
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
