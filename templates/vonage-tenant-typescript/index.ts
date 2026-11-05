import * as pulumi from "@pulumi/pulumi";
import * as vonage from "@pulumi-labs/vonage";

const config = new pulumi.Config();
const tenantName = config.get("tenantName") ?? "tenant-acme";
const newApiSecret = config.requireSecret("newApiSecret");

const tenant = new vonage.SubAccount("tenant", {
    name: tenantName,
    usePrimaryAccountBalance: true,
    // Vonage cannot delete subaccounts; suspend on destroy so it stops being usable.
    suspendOnDelete: true,
});

const tenantApp = new vonage.Application("tenant-app", {
    name: `${tenantName}-voice`,
    voice: {
        answerUrl: { address: "https://example.com/webhooks/answer", httpMethod: "POST" },
        eventUrl: { address: "https://example.com/webhooks/event", httpMethod: "POST" },
    },
});

// An account can hold at most two API secrets: add the new one, roll consumers over,
// then remove the old one out of band.
const tenantSecret = new vonage.ApiSecret("tenant-secret", {
    accountKey: tenant.apiKey,
    secret: newApiSecret,
});

const lookup = tenantApp.name.apply(name => vonage.getApplicationOutput({ name }));

export const tenantApiKey = tenant.apiKey;
export const tenantGeneratedSecret = tenant.generatedSecret;
export const applicationId = tenantApp.applicationId;
export const lookedUpApplicationId = lookup.applicationId;
export const secretId = tenantSecret.secretId;
