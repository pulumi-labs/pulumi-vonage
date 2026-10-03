package com.pulumi.example;

import com.pulumi.Context;
import com.pulumi.Pulumi;
import com.pulumi.vonage.Application;
import com.pulumi.vonage.ApplicationArgs;
import com.pulumi.vonage.Number;
import com.pulumi.vonage.NumberArgs;
import com.pulumi.vonage.inputs.MessagesCapabilityArgs;
import com.pulumi.vonage.inputs.VoiceCapabilityArgs;
import com.pulumi.vonage.inputs.VoiceWebhookArgs;
import com.pulumi.vonage.inputs.WebhookArgs;

public class App {
    public static void main(String[] args) {
        Pulumi.run(App::stack);
    }

    private static void stack(Context ctx) {
        // Credentials come from `pulumi config set vonage:apiKey ...` / `vonage:apiSecret --secret`
        // or the VONAGE_API_KEY / VONAGE_API_SECRET environment variables.
        var config = ctx.config();
        var answerUrl = config.get("answerUrl").orElse("https://example.com/webhooks/answer");
        var eventUrl = config.get("eventUrl").orElse("https://example.com/webhooks/event");
        var country = config.get("country").orElse("US");

        var app = new Application("app", ApplicationArgs.builder()
            .name("pulumi-voice-and-messages")
            .voice(VoiceCapabilityArgs.builder()
                .answerUrl(VoiceWebhookArgs.builder().address(answerUrl).httpMethod("POST").build())
                .eventUrl(VoiceWebhookArgs.builder().address(eventUrl).httpMethod("POST").build())
                .signedCallbacks(true)
                .build())
            .messages(MessagesCapabilityArgs.builder()
                .version("v1")
                .inboundUrl(WebhookArgs.builder().address("https://example.com/webhooks/inbound").httpMethod("POST").build())
                .statusUrl(WebhookArgs.builder().address("https://example.com/webhooks/status").httpMethod("POST").build())
                .build())
            .build());

        // Adopt a number the account already owns and route it to the application.
        // cancelOnDelete=false: destroying the stack only forgets the number, it keeps billing.
        config.get("msisdn").ifPresent(msisdn -> new Number("number", NumberArgs.builder()
            .country(country)
            .msisdn(msisdn)
            .acquisition("adopt")
            .cancelOnDelete(false)
            .appId(app.applicationId())
            .build()));

        ctx.export("applicationId", app.applicationId());
        // Returned exactly once by Vonage and stored as a secret.
        ctx.export("privateKey", app.privateKey());
    }
}
