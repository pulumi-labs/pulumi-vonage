package main

import (
	"github.com/pulumi/pulumi-vonage/sdk/go/vonage"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Credentials come from `pulumi config set vonage:apiKey ...` / `vonage:apiSecret --secret`
		// or the VONAGE_API_KEY / VONAGE_API_SECRET environment variables.
		cfg := config.New(ctx, "")
		answerURL := cfg.Get("answerUrl")
		if answerURL == "" {
			answerURL = "https://example.com/webhooks/answer"
		}
		eventURL := cfg.Get("eventUrl")
		if eventURL == "" {
			eventURL = "https://example.com/webhooks/event"
		}
		country := cfg.Get("country")
		if country == "" {
			country = "US"
		}

		app, err := vonage.NewApplication(ctx, "app", &vonage.ApplicationArgs{
			Name: pulumi.String("pulumi-voice-and-messages"),
			Voice: &vonage.VoiceCapabilityArgs{
				AnswerUrl:       &vonage.VoiceWebhookArgs{Address: pulumi.String(answerURL), HttpMethod: pulumi.String("POST")},
				EventUrl:        &vonage.VoiceWebhookArgs{Address: pulumi.String(eventURL), HttpMethod: pulumi.String("POST")},
				SignedCallbacks: pulumi.Bool(true),
			},
			Messages: &vonage.MessagesCapabilityArgs{
				Version:    pulumi.String("v1"),
				InboundUrl: &vonage.WebhookArgs{Address: pulumi.String("https://example.com/webhooks/inbound"), HttpMethod: pulumi.String("POST")},
				StatusUrl:  &vonage.WebhookArgs{Address: pulumi.String("https://example.com/webhooks/status"), HttpMethod: pulumi.String("POST")},
			},
		})
		if err != nil {
			return err
		}

		// Adopt a number the account already owns and route it to the application.
		// CancelOnDelete=false: destroying the stack only forgets the number, it keeps billing.
		if msisdn := cfg.Get("msisdn"); msisdn != "" {
			if _, err := vonage.NewNumber(ctx, "number", &vonage.NumberArgs{
				Country:        pulumi.String(country),
				Msisdn:         pulumi.String(msisdn),
				Acquisition:    pulumi.String("adopt"),
				CancelOnDelete: pulumi.Bool(false),
				AppId:          app.ApplicationId,
			}); err != nil {
				return err
			}
		}

		ctx.Export("applicationId", app.ApplicationId)
		// Returned exactly once by Vonage and stored as a secret.
		ctx.Export("privateKey", app.PrivateKey)
		return nil
	})
}
