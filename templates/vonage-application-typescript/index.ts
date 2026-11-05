import * as pulumi from "@pulumi/pulumi";
import * as vonage from "@pulumi-labs/vonage";

// Credentials come from `pulumi config set vonage:apiKey ...` / `vonage:apiSecret --secret`
// or the VONAGE_API_KEY / VONAGE_API_SECRET environment variables.
const config = new pulumi.Config();
const answerUrl = config.get("answerUrl") ?? "https://example.com/webhooks/answer";
const eventUrl = config.get("eventUrl") ?? "https://example.com/webhooks/event";
const msisdn = config.get("msisdn");
const country = config.get("country") ?? "US";

const app = new vonage.Application("app", {
    name: "pulumi-voice-and-messages",
    voice: {
        answerUrl: { address: answerUrl, httpMethod: "POST" },
        eventUrl: { address: eventUrl, httpMethod: "POST" },
        signedCallbacks: true,
    },
    messages: {
        version: "v1",
        inboundUrl: { address: "https://example.com/webhooks/inbound", httpMethod: "POST" },
        statusUrl: { address: "https://example.com/webhooks/status", httpMethod: "POST" },
    },
});

// Adopt a number the account already owns and route it to the application.
// cancelOnDelete=false: destroying the stack only forgets the number, it keeps billing.
if (msisdn) {
    new vonage.Number("number", {
        country: country,
        msisdn: msisdn,
        acquisition: "adopt",
        cancelOnDelete: false,
        appId: app.applicationId,
    });
}

export const applicationId = app.applicationId;
// Returned exactly once by Vonage and stored as a secret.
export const privateKey = app.privateKey;
