using Pulumi;
using Pulumi.Labs.Vonage;
using Pulumi.Labs.Vonage.Inputs;

return await Deployment.RunAsync(() =>
{
    // Credentials come from `pulumi config set vonage:apiKey ...` / `vonage:apiSecret --secret`
    // or the VONAGE_API_KEY / VONAGE_API_SECRET environment variables.
    var config = new Pulumi.Config();
    var answerUrl = config.Get("answerUrl") ?? "https://example.com/webhooks/answer";
    var eventUrl = config.Get("eventUrl") ?? "https://example.com/webhooks/event";
    var msisdn = config.Get("msisdn");
    var country = config.Get("country") ?? "US";

    var app = new Application("app", new ApplicationArgs
    {
        Name = "pulumi-voice-and-messages",
        Voice = new VoiceCapabilityArgs
        {
            AnswerUrl = new VoiceWebhookArgs { Address = answerUrl, HttpMethod = "POST" },
            EventUrl = new VoiceWebhookArgs { Address = eventUrl, HttpMethod = "POST" },
            SignedCallbacks = true,
        },
        Messages = new MessagesCapabilityArgs
        {
            Version = "v1",
            InboundUrl = new WebhookArgs { Address = "https://example.com/webhooks/inbound", HttpMethod = "POST" },
            StatusUrl = new WebhookArgs { Address = "https://example.com/webhooks/status", HttpMethod = "POST" },
        },
    });

    // Adopt a number the account already owns and route it to the application.
    // CancelOnDelete=false: destroying the stack only forgets the number, it keeps billing.
    if (!string.IsNullOrEmpty(msisdn))
    {
        new Pulumi.Labs.Vonage.Number("number", new NumberArgs
        {
            Country = country,
            Msisdn = msisdn,
            Acquisition = "adopt",
            CancelOnDelete = false,
            AppId = app.ApplicationId,
        });
    }

    return new Dictionary<string, object?>
    {
        ["applicationId"] = app.ApplicationId,
        // Returned exactly once by Vonage and stored as a secret.
        ["privateKey"] = app.PrivateKey,
    };
});
