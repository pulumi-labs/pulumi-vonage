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
func newProvider() (provider.Provider, error) {
	return infer.NewProviderBuilder().
		WithDisplayName("Vonage").
		WithDescription(
			"Experimental: manage Vonage (formerly Nexmo) applications, numbers, subaccounts, API secrets, Verify v2 templates and Network Registry business profiles as infrastructure with Pulumi.",
		).
		WithHomepage("https://developer.vonage.com").
		WithRepository("https://github.com/pulumi-labs/pulumi-vonage").
		WithPublisher("Pulumi Labs").
		WithLanguageMap(map[string]any{
			"go": map[string]any{"generateResourceContainerTypes": true, "respectSchemaVersion": true},
			"nodejs": map[string]any{
				"packageName":          "@pulumi-labs/vonage",
				"packageDescription":   "Experimental: manage Vonage (formerly Nexmo) applications, numbers, subaccounts, API secrets, Verify v2 templates and Network Registry business profiles as infrastructure with Pulumi.",
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
			infer.Resource[*AccountSettings, AccountSettingsArgs, AccountSettingsState](&AccountSettings{}),
			infer.Resource[*ApiSecret, ApiSecretArgs, ApiSecretState](&ApiSecret{}),
			infer.Resource[*VerifyTemplate, VerifyTemplateArgs, VerifyTemplateState](&VerifyTemplate{}),
			infer.Resource[*VerifyTemplateFragment, VerifyTemplateFragmentArgs, VerifyTemplateFragmentState](&VerifyTemplateFragment{}),
			infer.Resource[*NumberPool, NumberPoolArgs, NumberPoolState](&NumberPool{}),
			infer.Resource[*NumberPoolNumber, NumberPoolNumberArgs, NumberPoolNumberState](&NumberPoolNumber{}),
			infer.Resource[*User, UserArgs, UserState](&User{}),
			infer.Resource[*MessengerAccount, MessengerAccountArgs, MessengerAccountState](&MessengerAccount{}),
			infer.Resource[*ExternalAccountApplicationLink, ExternalAccountApplicationLinkArgs, ExternalAccountApplicationLinkState](&ExternalAccountApplicationLink{}),
			infer.Resource[*NetworkBusinessProfile, NetworkBusinessProfileArgs, NetworkBusinessProfileState](&NetworkBusinessProfile{}),
			infer.Resource[*NetworkApplicationProfile, NetworkApplicationProfileArgs, NetworkApplicationProfileState](&NetworkApplicationProfile{}),
			infer.Resource[*NetworkApplicationProfileShare, NetworkApplicationProfileShareArgs, NetworkApplicationProfileShareState](&NetworkApplicationProfileShare{}),
			infer.Resource[*TenDlcAccountSettings, TenDlcAccountSettingsArgs, TenDlcAccountSettingsState](&TenDlcAccountSettings{}),
			infer.Resource[*TenDlcBrand, TenDlcBrandArgs, TenDlcBrandState](&TenDlcBrand{}),
			infer.Resource[*TenDlcCampaign, TenDlcCampaignArgs, TenDlcCampaignState](&TenDlcCampaign{}),
			infer.Resource[*TenDlcCampaignNumber, TenDlcCampaignNumberArgs, TenDlcCampaignNumberState](&TenDlcCampaignNumber{}),
			infer.Resource[*TenDlcReseller, TenDlcResellerArgs, TenDlcResellerState](&TenDlcReseller{}),
		).
		WithFunctions(
			infer.Function[*GetApplication, GetApplicationArgs, GetApplicationResult](&GetApplication{}),
			infer.Function[*GetNumber, GetNumberArgs, GetNumberResult](&GetNumber{}),
			infer.Function[*GetApplications, GetApplicationsArgs, GetApplicationsResult](&GetApplications{}),
			infer.Function[*GetSubAccount, GetSubAccountArgs, GetSubAccountResult](&GetSubAccount{}),
			infer.Function[*GetSubAccounts, GetSubAccountsArgs, GetSubAccountsResult](&GetSubAccounts{}),
			infer.Function[*GetAccountSettings, GetAccountSettingsArgs, GetAccountSettingsResult](&GetAccountSettings{}),
			infer.Function[*GetAccountBalance, GetAccountBalanceArgs, GetAccountBalanceResult](&GetAccountBalance{}),
			infer.Function[*GetNumbers, GetNumbersArgs, GetNumbersResult](&GetNumbers{}),
			infer.Function[*GetAvailableNumbers, GetAvailableNumbersArgs, GetAvailableNumbersResult](&GetAvailableNumbers{}),
			infer.Function[*GetPricing, GetPricingArgs, GetPricingResult](&GetPricing{}),
			infer.Function[*GetApiSecret, GetApiSecretArgs, GetApiSecretResult](&GetApiSecret{}),
			infer.Function[*GetApiSecrets, GetApiSecretsArgs, GetApiSecretsResult](&GetApiSecrets{}),
			infer.Function[*GetVerifyTemplate, GetVerifyTemplateArgs, GetVerifyTemplateResult](&GetVerifyTemplate{}),
			infer.Function[*GetVerifyTemplateFragment, GetVerifyTemplateFragmentArgs, GetVerifyTemplateFragmentResult](&GetVerifyTemplateFragment{}),
			infer.Function[*GetNumberPool, GetNumberPoolArgs, GetNumberPoolResult](&GetNumberPool{}),
			infer.Function[*GetNumberPools, GetNumberPoolsArgs, GetNumberPoolsResult](&GetNumberPools{}),
			infer.Function[*GetNumberPoolNumber, GetNumberPoolNumberArgs, GetNumberPoolNumberResult](&GetNumberPoolNumber{}),
			infer.Function[*GetNumberPoolNumbers, GetNumberPoolNumbersArgs, GetNumberPoolNumbersResult](&GetNumberPoolNumbers{}),
			infer.Function[*GetUser, GetUserArgs, GetUserResult](&GetUser{}),
			infer.Function[*GetUsers, GetUsersArgs, GetUsersResult](&GetUsers{}),
			infer.Function[*GetExternalAccount, GetExternalAccountArgs, GetExternalAccountResult](&GetExternalAccount{}),
			infer.Function[*GetExternalAccounts, GetExternalAccountsArgs, GetExternalAccountsResult](&GetExternalAccounts{}),
			infer.Function[*GetExternalAccountApplicationLink, GetExternalAccountApplicationLinkArgs, GetExternalAccountApplicationLinkResult](&GetExternalAccountApplicationLink{}),
			infer.Function[*GetMessengerAccount, GetMessengerAccountArgs, GetExternalAccountResult](&GetMessengerAccount{}),
			infer.Function[*GetNetworkBusinessProfile, GetNetworkBusinessProfileArgs, GetNetworkBusinessProfileResult](&GetNetworkBusinessProfile{}),
			infer.Function[*GetNetworkBusinessProfiles, GetNetworkBusinessProfilesArgs, GetNetworkBusinessProfilesResult](&GetNetworkBusinessProfiles{}),
			infer.Function[*GetNetworkApplicationProfile, GetNetworkApplicationProfileArgs, GetNetworkApplicationProfileResult](&GetNetworkApplicationProfile{}),
			infer.Function[*GetNetworkApplicationProfiles, GetNetworkApplicationProfilesArgs, GetNetworkApplicationProfilesResult](&GetNetworkApplicationProfiles{}),
			infer.Function[*GetNetworkApplicationProfileShare, GetNetworkApplicationProfileShareArgs, GetNetworkApplicationProfileShareResult](&GetNetworkApplicationProfileShare{}),
			infer.Function[*GetNetworkApplicationProfileShares, GetNetworkApplicationProfileSharesArgs, GetNetworkApplicationProfileSharesResult](&GetNetworkApplicationProfileShares{}),
			infer.Function[*GetNetworkRegistryCatalogue, GetNetworkRegistryCatalogueArgs, GetNetworkRegistryCatalogueResult](&GetNetworkRegistryCatalogue{}),
			infer.Function[*GetNetworkTermsAndConditions, GetNetworkTermsAndConditionsArgs, GetNetworkTermsAndConditionsResult](&GetNetworkTermsAndConditions{}),
			infer.Function[*GetTenDlcAccountSettings, GetTenDlcAccountSettingsArgs, GetTenDlcAccountSettingsResult](&GetTenDlcAccountSettings{}),
			infer.Function[*GetTenDlcBrand, GetTenDlcBrandArgs, TenDlcBrandInfo](&GetTenDlcBrand{}),
			infer.Function[*GetTenDlcBrands, GetTenDlcBrandsArgs, GetTenDlcBrandsResult](&GetTenDlcBrands{}),
			infer.Function[*GetTenDlcCampaign, GetTenDlcCampaignArgs, TenDlcCampaignInfo](&GetTenDlcCampaign{}),
			infer.Function[*GetTenDlcCampaigns, GetTenDlcCampaignsArgs, GetTenDlcCampaignsResult](&GetTenDlcCampaigns{}),
			infer.Function[*GetTenDlcCampaignNumber, GetTenDlcCampaignNumberArgs, TenDlcCampaignNumberInfo](&GetTenDlcCampaignNumber{}),
			infer.Function[*GetTenDlcCampaignNumbers, GetTenDlcCampaignNumbersArgs, GetTenDlcCampaignNumbersResult](&GetTenDlcCampaignNumbers{}),
			infer.Function[*GetTenDlcReseller, GetTenDlcResellerArgs, TenDlcResellerInfo](&GetTenDlcReseller{}),
			infer.Function[*GetTenDlcResellers, GetTenDlcResellersArgs, GetTenDlcResellersResult](&GetTenDlcResellers{}),
			infer.Function[*GetTenDlcCampaignTermsAndConditions, GetTenDlcCampaignTermsAndConditionsArgs, GetTenDlcCampaignTermsAndConditionsResult](&GetTenDlcCampaignTermsAndConditions{}),
			infer.Function[*GetTenDlcThroughputs, GetTenDlcThroughputsArgs, GetTenDlcThroughputsResult](&GetTenDlcThroughputs{}),
			infer.Function[*GetTenDlcEnums, GetTenDlcEnumsArgs, GetTenDlcEnumsResult](&GetTenDlcEnums{}),
		).
		Build()
}
