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
			"Experimental: manage Vonage (formerly Nexmo) applications, numbers, subaccounts, API secrets and Verify v2 templates as infrastructure with Pulumi.",
		).
		WithHomepage("https://developer.vonage.com").
		WithRepository("https://github.com/pulumi-labs/pulumi-vonage").
		WithPublisher("Pulumi Labs").
		WithLanguageMap(map[string]any{
			"go": map[string]any{"generateResourceContainerTypes": true, "respectSchemaVersion": true},
			"nodejs": map[string]any{
				"packageName":          "@pulumi-labs/vonage",
				"packageDescription":   "Experimental: manage Vonage (formerly Nexmo) applications, numbers, subaccounts, API secrets and Verify v2 templates as infrastructure with Pulumi.",
				"respectSchemaVersion": true,
			},
			"python": map[string]any{
				"packageName":          "pulumi_labs_vonage",
				"respectSchemaVersion": true,
				"pyproject":            map[string]any{"enabled": true},
			},
			"csharp": map[string]any{"rootNamespace": "Pulumi.Labs", "respectSchemaVersion": true},
			"java":   map[string]any{"basePackage": "com.pulumi.labs", "respectSchemaVersion": true},
		}).
		WithGoImportPath("github.com/pulumi-labs/pulumi-vonage/sdk/go/vonage").
		WithPluginDownloadURL("github://api.github.com/pulumi-labs/pulumi-vonage").
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
			infer.Resource[*VerifyTemplate, VerifyTemplateArgs, VerifyTemplateState](&VerifyTemplate{}),
			infer.Resource[*VerifyTemplateFragment, VerifyTemplateFragmentArgs, VerifyTemplateFragmentState](&VerifyTemplateFragment{}),
		).
		WithFunctions(
			infer.Function[*GetApplication, GetApplicationArgs, GetApplicationResult](&GetApplication{}),
			infer.Function[*GetNumber, GetNumberArgs, GetNumberResult](&GetNumber{}),
			infer.Function[*GetApplications, GetApplicationsArgs, GetApplicationsResult](&GetApplications{}),
			infer.Function[*GetSubAccount, GetSubAccountArgs, GetSubAccountResult](&GetSubAccount{}),
			infer.Function[*GetApiSecret, GetApiSecretArgs, GetApiSecretResult](&GetApiSecret{}),
			infer.Function[*GetApiSecrets, GetApiSecretsArgs, GetApiSecretsResult](&GetApiSecrets{}),
			infer.Function[*GetVerifyTemplate, GetVerifyTemplateArgs, GetVerifyTemplateResult](&GetVerifyTemplate{}),
			infer.Function[*GetVerifyTemplateFragment, GetVerifyTemplateFragmentArgs, GetVerifyTemplateFragmentResult](&GetVerifyTemplateFragment{}),
		).
		Build()
}
