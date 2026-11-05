package com.pulumi.example;

import com.pulumi.Context;
import com.pulumi.Pulumi;
import com.pulumi.labs.vonage.ApiSecret;
import com.pulumi.labs.vonage.ApiSecretArgs;
import com.pulumi.labs.vonage.Application;
import com.pulumi.labs.vonage.ApplicationArgs;
import com.pulumi.labs.vonage.SubAccount;
import com.pulumi.labs.vonage.SubAccountArgs;
import com.pulumi.labs.vonage.VonageFunctions;
import com.pulumi.labs.vonage.inputs.GetApplicationArgs;
import com.pulumi.labs.vonage.inputs.VoiceCapabilityArgs;
import com.pulumi.labs.vonage.inputs.VoiceWebhookArgs;

public class App {
    public static void main(String[] args) {
        Pulumi.run(App::stack);
    }

    private static void stack(Context ctx) {
        var config = ctx.config();
        var tenantName = config.get("tenantName").orElse("tenant-acme");
        var newApiSecret = config.requireSecret("newApiSecret");

        var tenant = new SubAccount("tenant", SubAccountArgs.builder()
            .name(tenantName)
            .usePrimaryAccountBalance(true)
            // Vonage cannot delete subaccounts; suspend on destroy so it stops being usable.
            .suspendOnDelete(true)
            .build());

        var tenantApp = new Application("tenant-app", ApplicationArgs.builder()
            .name(tenantName + "-voice")
            .voice(VoiceCapabilityArgs.builder()
                .answerUrl(VoiceWebhookArgs.builder().address("https://example.com/webhooks/answer").httpMethod("POST").build())
                .eventUrl(VoiceWebhookArgs.builder().address("https://example.com/webhooks/event").httpMethod("POST").build())
                .build())
            .build());

        // An account can hold at most two API secrets: add the new one, roll consumers over,
        // then remove the old one out of band.
        var tenantSecret = new ApiSecret("tenant-secret", ApiSecretArgs.builder()
            .accountKey(tenant.apiKey())
            .secret(newApiSecret)
            .build());

        var lookup = tenantApp.name().apply(name ->
            VonageFunctions.getApplication(GetApplicationArgs.builder().name(name).build()));

        ctx.export("tenantApiKey", tenant.apiKey());
        ctx.export("tenantGeneratedSecret", tenant.generatedSecret());
        ctx.export("applicationId", tenantApp.applicationId());
        ctx.export("lookedUpApplicationId", lookup.applyValue(r -> r.applicationId()));
        ctx.export("secretId", tenantSecret.secretId());
    }
}
