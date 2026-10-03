import pulumi
import pulumi_vonage as vonage

config = pulumi.Config()
tenant_name = config.get("tenantName") or "tenant-acme"
new_api_secret = config.require_secret("newApiSecret")

tenant = vonage.SubAccount(
    "tenant",
    name=tenant_name,
    use_primary_account_balance=True,
    # Vonage cannot delete subaccounts; suspend on destroy so it stops being usable.
    suspend_on_delete=True,
)

tenant_app = vonage.Application(
    "tenant-app",
    name=f"{tenant_name}-voice",
    voice={
        "answer_url": {"address": "https://example.com/webhooks/answer", "http_method": "POST"},
        "event_url": {"address": "https://example.com/webhooks/event", "http_method": "POST"},
    },
)

# An account can hold at most two API secrets: add the new one, roll consumers over,
# then remove the old one out of band.
tenant_secret = vonage.ApiSecret(
    "tenant-secret",
    account_key=tenant.api_key,
    secret=new_api_secret,
)

lookup = tenant_app.name.apply(lambda n: vonage.get_application_output(name=n))

pulumi.export("tenantApiKey", tenant.api_key)
pulumi.export("tenantGeneratedSecret", tenant.generated_secret)
pulumi.export("applicationId", tenant_app.application_id)
pulumi.export("lookedUpApplicationId", lookup.application_id)
pulumi.export("secretId", tenant_secret.secret_id)
