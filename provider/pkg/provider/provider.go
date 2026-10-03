// Package provider implements the Vonage Pulumi provider.
package provider

import (
	"github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// New builds the Vonage provider.
func New() (provider.Provider, error) {
	return infer.NewProviderBuilder().
		WithDisplayName("Vonage").
		WithDescription(
			"Manage Vonage (formerly Nexmo) applications, numbers, subaccounts and API secrets as infrastructure with Pulumi.",
		).
		WithHomepage("https://developer.vonage.com").
		WithRepository("https://github.com/pulumi/autoregistry").
		WithGoImportPath("github.com/pulumi/pulumi-vonage/sdk/go/vonage").
		WithPublisher("Pulumi").
		WithLicense("Apache-2.0").
		WithKeywords("pulumi", "vonage", "nexmo", "communications", "sms", "voice", "category/network").
		WithModuleMap(map[tokens.ModuleName]tokens.ModuleName{
			"provider": "index",
		}).
		WithConfig(infer.Config(Config{})).
		WithResources(
			infer.Resource[*Application, ApplicationArgs, ApplicationState](&Application{}),
			infer.Resource[*Number, NumberArgs, NumberState](&Number{}),
			infer.Resource[*SubAccount, SubAccountArgs, SubAccountState](&SubAccount{}),
			infer.Resource[*ApiSecret, ApiSecretArgs, ApiSecretState](&ApiSecret{}),
		).
		WithFunctions(
			infer.Function[*GetApplication, GetApplicationArgs, GetApplicationResult](&GetApplication{}),
			infer.Function[*GetNumber, GetNumberArgs, GetNumberResult](&GetNumber{}),
		).
		Build()
}
