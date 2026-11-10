# Vonage Pulumi Provider

> [!NOTE]
> **Experimental.** This provider is experimental, versions are `0.x`, and its API may change without notice.

A native Pulumi provider for [Vonage](https://developer.vonage.com) (formerly Nexmo). Vonage has no Terraform or Pulumi provider; this one manages the account-level building blocks of a Vonage estate as infrastructure: applications, virtual numbers, subaccounts, API secrets and Verify v2 templates.

## Resources

| Token | What it manages |
| --- | --- |
| `vonage:index:Application` | Application API v2 application with voice, messages, rtc, vbc, meetings and verify capabilities and their webhooks. A Vonage-generated private key is returned once as the secret output `privateKey`. |
| `vonage:index:Number` | An owned virtual number: buy or adopt, link to an application or webhooks, optionally cancel on destroy. |
| `vonage:index:NumberPool` | A number pool (`poolId`, `fallback`, `localizedSenderPreferred`, `predictableSenderPreferred`). Import with the pool ID. Shapes follow the Number Pools OpenAPI definition and are unverified against a live account. |
| `vonage:index:MessengerAccount` | A Facebook Messenger page registered with Vonage (`externalId`, secret `accessToken`, `name`; beta External Accounts API). Import with the Page ID. Unverified against a live account. |
| `vonage:index:ExternalAccountApplicationLink` | Links an application to a Messenger, Viber Service Message or WhatsApp account (`channel`, `externalId`, `applicationId`; one application per account); destroy only unlinks. Import with `CHANNEL/EXTERNAL_ID/APPLICATION_ID`. |
| `vonage:index:NetworkApplicationProfile` | A Network Registry application profile: which Network APIs an application uses and why (business profile, name, category, Network APIs with purposes, infrastructure locations). Reviewed by operators, so `status` moves from `pending` on its own. The API has no update call, so every input change replaces the profile (new one created first; each account may hold 50). Import with the profile ID. Unverified against a live account. |
| `vonage:index:NetworkApplicationProfileShare` | Shares an application profile with another Vonage account (`profileId`, `accountId`). Vonage accepts the share asynchronously, so a refresh right after creation may briefly not list it. Either input replaces it. Unverified against a live account. |
| `vonage:index:NetworkBusinessProfile` | A Network Registry business profile (countries, contact, organisation type, tax number, privacy policy, postal address). Reviewed by Vonage and operators, so `status` moves from `pending` on its own; a changed input is sent as a change request. Import with the profile ID. Shapes follow the Network Registry OpenAPI definition and are unverified against a live account. |
| `vonage:index:NumberPoolNumber` | One owned number in a pool; destroy only removes it from the pool. Import with `POOL_ID/NUMBER`. |
| `vonage:index:SubAccount` | A subaccount under a primary account (primary credentials required). Vonage cannot delete subaccounts, so destroy can suspend them. |
| `vonage:index:AccountSettings` | The default SMS webhooks of the account (`moCallBackUrl` for inbound SMS, `drCallBackUrl` for delivery receipts) and, as outputs, its rate limits. A singleton: create adopts the account's one settings record, the import ID is the API key, and destroy clears only the URLs the program manages. Vonage requests each URL and saves it only if it answers 200. Unverified against a live account. |
| `vonage:index:ApiSecret` | One API secret of an account or subaccount (at most two per account). Rotate by adding a second `ApiSecret` (blue/green), or change `secret` in place (see below). |
| `vonage:index:User` | A Users API user (`name`, `displayName`, `imageUrl`, `customData`, `channels`). Authenticates with an application JWT, so it needs `applicationId` and `privateKey` and not `apiKey`/`apiSecret`. Import with the user ID (`USR-...`). Unverified against a live account. |
| `vonage:index:VerifyTemplate` | A Verify v2 template (`name`, `isDefault`). Import with the template ID. |
| `vonage:index:VerifyTemplateFragment` | The text of a template for one `channel` (`sms` or `voice`) and `locale`. `templateId`, `channel` and `locale` replace on change; `text` updates in place. Import with `TEMPLATE_ID/FRAGMENT_ID`. |

Import IDs: `Application` the application ID; `Number` `COUNTRY:MSISDN`; `NumberPool` the pool ID; `NumberPoolNumber` `POOL_ID/NUMBER`; `NetworkBusinessProfile` the profile ID; `NetworkApplicationProfile` the profile ID; `NetworkApplicationProfileShare` `PROFILE_ID/ACCOUNT_ID`; `MessengerAccount` the Facebook Page ID; `ExternalAccountApplicationLink` `CHANNEL/EXTERNAL_ID/APPLICATION_ID`; `SubAccount` the subaccount API key; `ApiSecret` `ACCOUNT_API_KEY/SECRET_ID`; `AccountSettings` the account API key; `User` the user ID; `VerifyTemplate` the template ID; `VerifyTemplateFragment` `TEMPLATE_ID/FRAGMENT_ID`.

Verify v2 templates are documented for Basic and JWT authentication; the Vonage Node SDK uses a JWT. The provider sends Basic, which is unverified against a live account.

Data sources (`vonage:index:get*`): `getApplication` (by `applicationId` or exact `name`; returns the full capability tree, not only the names), `getApplications`, `getNumber` (by `country` and `msisdn`), `getSubAccount` (by `subAccountApiKey`), `getSubAccounts` (optionally by exact name), `getAccountBalance`, `getAccountSettings` (webhooks and rate limits; Vonage has no GET, so it sends an empty POST to `/account/settings`), `getNumbers` (owned numbers, filtered by country, pattern, application), `getAvailableNumbers` (numbers for sale in a country; read-only, never buys), `getNumberPool`, `getNumberPools`, `getNumberPoolNumber` (is a number in a pool) and `getNumberPoolNumbers` (pools and their numbers; read-only), `getUser` (by `userId` or exact `name`) and `getUsers` (every user, or the one named; JWT-authenticated), `getExternalAccount` (by `channel` and `externalId`), `getExternalAccountApplicationLink` (is an application linked), `getExternalAccounts` (every account, or one channel; no tokens) and `getMessengerAccount` (by Page ID; returns the secret page access token), `getNetworkApplicationProfile`, `getNetworkApplicationProfiles`, `getNetworkApplicationProfileShare`, `getNetworkApplicationProfileShares`, `getNetworkBusinessProfile` (by `profileId`), `getNetworkBusinessProfiles` (own or `shared`), `getNetworkRegistryCatalogue` (countries, organisation types, categories, CSPs, Network APIs, purposes, vetting levels, terms and conditions names; optional `countries` and `all` filters) and `getNetworkTermsAndConditions` (full text by `documentId`), `getPricing` (outbound sms, sms-transit or voice prices for a country, a dialling prefix or every country; read-only, prices are decimal strings), `getApiSecret` and `getApiSecrets` (secret metadata only, Vonage never returns the value), `getVerifyTemplate` (by `templateId` or exact `name`) and `getVerifyTemplateFragment` (by `templateFragmentId`, or `channel` and `locale`). The full vendor surface and what is deferred is in `research/coverage/vonage.md`.

## Configuration

| Property | Env var | Notes |
| --- | --- | --- |
| `apiKey` | `VONAGE_API_KEY` | Account API key. |
| `apiSecret` | `VONAGE_API_SECRET` | Secret. Account API secret (HTTP Basic). |
| `applicationId` | `VONAGE_APPLICATION_ID` | Application ID the JWT is signed for. Required by `User`, `getUser` and `getUsers` (users belong to this application); optional otherwise. |
| `privateKey` | `VONAGE_PRIVATE_KEY` | Secret. PEM private key of that application (PKCS#8 or PKCS#1; a value with newlines flattened to `\n` is accepted). |
| `apiBaseUrl` | `VONAGE_API_BASE_URL` | Override for `https://api.nexmo.com` (tests, proxies). |
| `restBaseUrl` | `VONAGE_REST_BASE_URL` | Override for `https://rest.nexmo.com`. |

Most resources use the account API key and secret over HTTP Basic. `User`, `getUser` and `getUsers` call the Users API, which only accepts a JWT, so they use `applicationId` and `privateKey` instead and do not need `apiKey` and `apiSecret`. A JWT is signed locally (RS256, 15 minutes) for every request; the private key never leaves the provider. Each resource names the credential it is missing.

```bash
pulumi config set vonage:apiKey <key>
pulumi config set --secret vonage:apiSecret <secret>
pulumi config set vonage:applicationId <id>                  # optional
pulumi config set --secret vonage:privateKey <pem>           # optional
```

Or set the environment variables instead (config takes precedence when both are set):

```bash
export VONAGE_API_KEY=<key>
export VONAGE_API_SECRET=<secret>
```

## Validation status

This provider has not been validated against a live Vonage account. The API shapes (endpoints, request bodies, response fields) come from Vonage's published API documentation and SDKs and are exercised only against an in-process mock server in the unit tests. Expect to adjust details when first run against real credentials, and try it on a test account before relying on it.

## Billing and destructive semantics

* `Number` with `acquisition: buy` purchases a number, which is **billed**. Purchases are never retried automatically after a transport error or 5xx, because the first attempt may have been applied. If a purchase ends in such an ambiguous error, the provider checks the account once more; when the number is now owned it is tracked in state with an init-failed error (so Pulumi can configure or cancel it), otherwise the plain error is returned. A definitive refusal (HTTP 4xx or a Vonage error code) is never treated as a purchase.
* `Number` requires an explicit `cancelOnDelete`. `true` means `pulumi destroy` permanently cancels (releases) the number. `false` means destroy only forgets it in Pulumi state and the number keeps billing.
* `acquisition: adopt` manages a number the account already owns and fails if it is not owned; use it to bring existing numbers under management without any purchase.
* `Number.acquisition` is create-only: it is read when the resource is created and later changes are ignored (no diff, no replacement). Replacing a number would create the replacement first and then delete the old resource, which cancels the number when `cancelOnDelete` is true. To really buy a different number, change `country` or `msisdn`, which replaces the resource.
* `Number` import IDs are `COUNTRY:MSISDN`, for example `US:14155550100`; an ID without the country is rejected.
* `SubAccount.suspendOnDelete: true` suspends the subaccount on destroy; there is no delete API. With false or unset the subaccount is only removed from state.
* `ApiSecret` destroy revokes the secret. Vonage allows two secrets per account and refuses to revoke the last one.
* `ApiSecret` import: the ID is `ACCOUNT_API_KEY/SECRET_ID`. Vonage never returns a secret value, so an imported secret has no value in state. Set `secret` in your program to the real value and the first `pulumi up` records it in place (no Vonage call, no replacement, nothing revoked); changing it afterwards replaces the resource.
* `ApiSecret` rotation and the two-secret limit: an account holds at most two secrets, and an account that still has its original secret plus one managed secret is already at the limit. When you change `secret` and the account holds two secrets, the replacement revokes the old managed secret first and then creates the new one (the original secret keeps working throughout, but the revoked value stops working immediately). When the account holds only one secret, the new secret is created first and then the old one is revoked. For rotation with no cut-over window, manage two `ApiSecret` resources (blue and green) and swap which one your consumers use. Failure mode: with delete-before-replace the old secret stops working at once, so if creating the new secret then fails, consumers of the old value are broken until you fix the program and re-run. Whether the replacement is delete-first is decided in `Diff`, which lists the account's secrets (a network call), so a preview and the later apply can disagree if the secret count changes in between; if the listing fails, the create-then-revoke order is used with a warning. The two-secret limit comes from Vonage's documentation and has not been verified live.
* `SubAccount.secret` is create-only: the Vonage subaccount API cannot change it later, so edits are ignored (no diff). Rotate with `ApiSecret`.
* `Number` voice routing (`voiceCallbackType`/`voiceCallbackValue`) and `voiceStatusCallback` are cleared on Vonage when removed from the program. `voiceStatusCallback` cannot be read back: the owned-numbers listing does not report it, so refresh keeps the declared value and drift in it is not detected.
* Webhook URLs (`Application` capability addresses, `Number` callback values) are stored in state and shown in plain text; if a URL embeds a token, treat it as sensitive and prefer header-based or signature verification on the receiving side.
* `apiBaseUrl` and `restBaseUrl` overrides must use `https` (plain `http` is accepted only for loopback hosts), because requests carry the account key and secret as HTTP Basic credentials.
* `Application.privateKey` is only available from the create response. Supply your own `publicKey` to avoid generating a key. A declared `publicKey` is refreshed from Vonage and compared as decoded PEM (line endings, wrapping and trailing whitespace are not drift), so a key rotated outside Pulumi shows up as drift and the next update restores the declared key; the registered key is always exposed as `applicationPublicKey`.
* `Application` updates replace the whole configuration. `video` and `networkApis` are declarable. When the program does not declare them, a capability that exists on the application is read before each update and sent back unchanged, so an update never strips one configured in the dashboard, and it is not shown in state. Declare the block to manage it; removing a block you declared earlier disables the capability on the next update. Capabilities the provider does not model at all (for example the sunset `bulk`) are preserved the same way.
* `Application` `messages.version` is write-only: Vonage does not return it on GET, so the value you declared is kept through refreshes. That Vonage omits `messages.version` on GET comes from the Vonage OpenAPI definition and SDK types and has not been verified live; the same holds for Vonage accepting `keys.public_key` in the application PUT, which only the in-process fake demonstrates.
* `SubAccount` `suspended` and `usePrimaryAccountBalance` are always reconciled: leaving them out of the program means the Vonage defaults (not suspended, uses the primary balance), a subaccount suspended outside Pulumi shows up as drift on refresh, and import adopts a non-default value. Removing `suspended: true` from the program unsuspends the subaccount.

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
* Account settings (default SMS webhooks; there is no read endpoint), balance auto-reload, and subaccount balance or number transfers (imperative operations rather than resources).
* More JWT-authenticated APIs (Messages sender settings, Video, Meetings rooms); the signer exists since `User`.
* SIP trunking.
* 10DLC and Network Registry (see `research/coverage/vonage.md`).

## Development

```bash
make tidy build test lint
make gen-sdk   # needs the pulumi CLI
```

API contracts were derived from the public Vonage API reference at developer.vonage.com and the official Vonage Python SDK (the OpenAPI definitions are not published as a stable public repository). Behaviour has been exercised against mocked HTTP servers only, not a live Vonage account.
