import pulumi
import pulumi_vonage as vonage

# Credentials come from `pulumi config set vonage:apiKey ...` / `vonage:apiSecret --secret`
# or the VONAGE_API_KEY / VONAGE_API_SECRET environment variables.
config = pulumi.Config()
answer_url = config.get("answerUrl") or "https://example.com/webhooks/answer"
event_url = config.get("eventUrl") or "https://example.com/webhooks/event"
msisdn = config.get("msisdn")
country = config.get("country") or "US"

app = vonage.Application(
    "app",
    name="pulumi-voice-and-messages",
    voice={
        "answer_url": {"address": answer_url, "http_method": "POST"},
        "event_url": {"address": event_url, "http_method": "POST"},
        "signed_callbacks": True,
    },
    messages={
        "version": "v1",
        "inbound_url": {"address": "https://example.com/webhooks/inbound", "http_method": "POST"},
        "status_url": {"address": "https://example.com/webhooks/status", "http_method": "POST"},
    },
)

# Adopt a number the account already owns and route it to the application.
# cancel_on_delete=False: destroying the stack only forgets the number, it keeps billing.
if msisdn:
    vonage.Number(
        "number",
        country=country,
        msisdn=msisdn,
        acquisition="adopt",
        cancel_on_delete=False,
        app_id=app.application_id,
    )

pulumi.export("applicationId", app.application_id)
# Returned exactly once by Vonage and stored as a secret.
pulumi.export("privateKey", app.private_key)
