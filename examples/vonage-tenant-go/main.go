package main

import (
	"fmt"

	"github.com/pulumi-labs/pulumi-vonage/sdk/go/vonage"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		tenantName := cfg.Get("tenantName")
		if tenantName == "" {
			tenantName = "tenant-acme"
		}
		newAPISecret := cfg.RequireSecret("newApiSecret")

		tenant, err := vonage.NewSubAccount(ctx, "tenant", &vonage.SubAccountArgs{
			Name:                     pulumi.String(tenantName),
			UsePrimaryAccountBalance: pulumi.Bool(true),
			// Vonage cannot delete subaccounts; suspend on destroy so it stops being usable.
			SuspendOnDelete: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		tenantApp, err := vonage.NewApplication(ctx, "tenant-app", &vonage.ApplicationArgs{
			Name: pulumi.String(fmt.Sprintf("%s-voice", tenantName)),
			Voice: &vonage.VoiceCapabilityArgs{
				AnswerUrl: &vonage.VoiceWebhookArgs{Address: pulumi.String("https://example.com/webhooks/answer"), HttpMethod: pulumi.String("POST")},
				EventUrl:  &vonage.VoiceWebhookArgs{Address: pulumi.String("https://example.com/webhooks/event"), HttpMethod: pulumi.String("POST")},
			},
		})
		if err != nil {
			return err
		}

		// An account can hold at most two API secrets: add the new one, roll consumers over,
		// then remove the old one out of band.
		tenantSecret, err := vonage.NewApiSecret(ctx, "tenant-secret", &vonage.ApiSecretArgs{
			AccountKey: tenant.ApiKey,
			Secret:     newAPISecret,
		})
		if err != nil {
			return err
		}

		lookup := tenantApp.Name.ApplyT(func(name string) (string, error) {
			res, err := vonage.LookupApplication(ctx, &vonage.LookupApplicationArgs{Name: &name})
			if err != nil {
				return "", err
			}
			return res.ApplicationId, nil
		}).(pulumi.StringOutput)

		ctx.Export("tenantApiKey", tenant.ApiKey)
		ctx.Export("tenantGeneratedSecret", tenant.GeneratedSecret)
		ctx.Export("applicationId", tenantApp.ApplicationId)
		ctx.Export("lookedUpApplicationId", lookup)
		ctx.Export("secretId", tenantSecret.SecretId)
		return nil
	})
}
