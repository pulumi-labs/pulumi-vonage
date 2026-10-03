using Pulumi;
using Pulumi.Vonage;
using Pulumi.Vonage.Inputs;

return await Deployment.RunAsync(() =>
{
    var config = new Pulumi.Config();
    var tenantName = config.Get("tenantName") ?? "tenant-acme";
    var newApiSecret = config.RequireSecret("newApiSecret");

    var tenant = new SubAccount("tenant", new SubAccountArgs
    {
        Name = tenantName,
        UsePrimaryAccountBalance = true,
        // Vonage cannot delete subaccounts; suspend on destroy so it stops being usable.
        SuspendOnDelete = true,
    });

    var tenantApp = new Application("tenant-app", new ApplicationArgs
    {
        Name = $"{tenantName}-voice",
        Voice = new VoiceCapabilityArgs
        {
            AnswerUrl = new VoiceWebhookArgs { Address = "https://example.com/webhooks/answer", HttpMethod = "POST" },
            EventUrl = new VoiceWebhookArgs { Address = "https://example.com/webhooks/event", HttpMethod = "POST" },
        },
    });

    // An account can hold at most two API secrets: add the new one, roll consumers over,
    // then remove the old one out of band.
    var tenantSecret = new ApiSecret("tenant-secret", new ApiSecretArgs
    {
        AccountKey = tenant.ApiKey,
        Secret = newApiSecret,
    });

    var lookup = tenantApp.Name.Apply(name => GetApplication.Invoke(new GetApplicationInvokeArgs { Name = name }));

    return new Dictionary<string, object?>
    {
        ["tenantApiKey"] = tenant.ApiKey,
        ["tenantGeneratedSecret"] = tenant.GeneratedSecret,
        ["applicationId"] = tenantApp.ApplicationId,
        ["lookedUpApplicationId"] = lookup.Apply(l => l.ApplicationId),
        ["secretId"] = tenantSecret.SecretId,
    };
});
