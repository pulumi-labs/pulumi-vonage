# Vonage Pulumi Provider

A native Pulumi provider for [Vonage](https://developer.vonage.com) (formerly Nexmo). Vonage has no Terraform or Pulumi provider; this one manages the account-level building blocks of a Vonage estate as infrastructure: applications, virtual numbers, subaccounts and API secrets.

## Resources

| Token | What it manages |
| --- | --- |
| `vonage:index:Application` | Application API v2 application with voice, messages, rtc, vbc, meetings and verify capabilities and their webhooks. A Vonage-generated private key is returned once as the secret output `privateKey`. |
| `vonage:index:Number` | An owned virtual number: buy or adopt, link to an application or webhooks, optionally cancel on destroy. |
| `vonage:index:SubAccount` | A subaccount under a primary account (primary credentials required). Vonage cannot delete subaccounts, so destroy can suspend them. |
| `vonage:index:ApiSecret` | One API secret of an account or subaccount (at most two per account), enabling zero-downtime rotation. |

Data sources: `vonage:index:getApplication` (by `applicationId` or exact `name`) and `vonage:index:getNumber` (by `country` and `msisdn`).

## Configuration

| Property | Env var | Notes |
| --- | --- | --- |
| `apiKey` | `VONAGE_API_KEY` | Account API key. |
| `apiSecret` | `VONAGE_API_SECRET` | Secret. Account API secret (HTTP Basic). |
| `applicationId` | `VONAGE_APPLICATION_ID` | Optional, reserved for JWT-authenticated APIs. |
| `privateKey` | `VONAGE_PRIVATE_KEY` | Optional secret, reserved for JWT-authenticated APIs. |
| `apiBaseUrl` | `VONAGE_API_BASE_URL` | Override for `https://api.nexmo.com` (tests, proxies). |
| `restBaseUrl` | `VONAGE_REST_BASE_URL` | Override for `https://rest.nexmo.com`. |

Every resource in this release uses the account API key and secret over HTTP Basic. `applicationId` and `privateKey` are accepted so one provider configuration can carry them, but nothing uses them yet.

```bash
pulumi config set vonage:apiKey <key>
pulumi config set --secret vonage:apiSecret <secret>
```

## Validation status

This provider has not been validated against a live Vonage account. The API shapes (endpoints, request bodies, response fields) come from Vonage's published API documentation and SDKs and are exercised only against an in-process mock server in the unit tests. Expect to adjust details when first run against real credentials, and try it on a test account before relying on it.

## Billing and destructive semantics

* `Number` with `acquisition: buy` purchases a number, which is **billed**. Purchases are never retried automatically after a transport error or 5xx, because the first attempt may have been applied.
* `Number` requires an explicit `cancelOnDelete`. `true` means `pulumi destroy` permanently cancels (releases) the number. `false` means destroy only forgets it in Pulumi state and the number keeps billing.
* `acquisition: adopt` manages a number the account already owns and fails if it is not owned; use it to bring existing numbers under management without any purchase.
* `SubAccount.suspendOnDelete: true` suspends the subaccount on destroy; there is no delete API. With false or unset the subaccount is only removed from state.
* `ApiSecret` destroy revokes the secret. Vonage allows two secrets per account and refuses to revoke the last one.
* `ApiSecret` import: Vonage never returns a secret value, so an imported secret has no value in state. Set `secret` in your program to the real value and the first `pulumi up` records it in place (no Vonage call, no replacement, nothing revoked); changing it afterwards replaces the resource (create new, then revoke old).
* `SubAccount.secret` is create-only: the Vonage subaccount API cannot change it later, so edits are ignored (no diff). Rotate with `ApiSecret`.
* `Number` voice routing (`voiceCallbackType`/`voiceCallbackValue`) and `voiceStatusCallback` are cleared on Vonage when removed from the program. Drift in `voiceStatusCallback` is detected on refresh only if Vonage reports it in the owned-numbers listing.
* Webhook URLs (`Application` capability addresses, `Number` callback values) are stored in state and shown in plain text; if a URL embeds a token, treat it as sensitive and prefer header-based or signature verification on the receiving side.
* `apiBaseUrl` and `restBaseUrl` overrides must use `https` (plain `http` is accepted only for loopback hosts), because requests carry the account key and secret as HTTP Basic credentials.
* `Application.privateKey` is only available from the create response. Supply your own `publicKey` to avoid generating a key.

## Example

```yaml
resources:
  app:
    type: vonage:index:Application
    properties:
      name: support-voice
      voice:
        answerUrl:
          address: https://example.com/webhooks/answer
          httpMethod: POST
        eventUrl:
          address: https://example.com/webhooks/event
          httpMethod: POST
      messages:
        version: v1
        inboundUrl:
          address: https://example.com/webhooks/inbound
        statusUrl:
          address: https://example.com/webhooks/status
  number:
    type: vonage:index:Number
    properties:
      country: US
      msisdn: "14155550100"
      acquisition: adopt
      cancelOnDelete: false
      appId: ${app.applicationId}
```

Examples in all six languages are in `examples/`, starter templates in `templates/`.

## Roadmap (not yet implemented)

* Number search-and-buy by criteria (this provider buys a specific, already-known msisdn).
* Account settings (default SMS webhooks), balance auto-reload, and subaccount balance or number transfers (imperative operations rather than resources).
* JWT-authenticated APIs (Messages sender settings, Video, Meetings rooms) using `applicationId` and `privateKey`.
* Verify v2 templates and SIP trunking.

## Development

```bash
make tidy build test lint
make gen-sdk   # needs the pulumi CLI
```

API contracts were derived from the public Vonage API reference at developer.vonage.com and the official Vonage Python SDK (the OpenAPI definitions are not published as a stable public repository). Behaviour has been exercised against mocked HTTP servers only, not a live Vonage account.
