# Changelog

## [Unreleased]

## [0.1.0] - 2026-11-04

### Added

- `VerifyTemplate` and `VerifyTemplateFragment` resources for Verify v2 templates and their per-channel, per-locale text (import IDs: the template ID, and `TEMPLATE_ID/FRAGMENT_ID`). Request shapes follow the Vonage Node SDK; Basic authentication on these endpoints is unverified against a live account.
- Lookup functions `getSubAccount`, `getApiSecret`, `getApiSecrets`, `getApplications`, `getVerifyTemplate` and `getVerifyTemplateFragment`, so every resource now has a lookup.
- `research/coverage/vonage.md`: the coverage inventory of the Vonage SDKs and APIs, with what is deferred and why.

### Changed

- `go.mod`/`go.sum` moved from `provider/` to the provider's top level (the monorepo module path now ends in `lib/providers/vonage`); the Makefile builds from there. No change to the provider binary, schema or SDKs.

### Fixed

- BREAKING: `Number.acquisition` is create-only. Changing it no longer replaces the number (the replacement had the same ID, so the engine created it first and then deleted the old resource, which cancelled the number when `cancelOnDelete` was true). Edits are ignored after create and the state keeps the mode that was actually used. `country` and `msisdn` still replace.
- BREAKING: `Number` import IDs must be `COUNTRY:MSISDN`; an ID without the country is rejected instead of recording an empty country that forced a replacement on the next `up`.
- `Number` create now reports a failed or empty read-back after the purchase as a partial-state init failure (the number is tracked) instead of silently falling back to the inputs. An update that changes only `cancelOnDelete` no longer calls the Numbers API.
- BREAKING (behaviour): `ApiSecret` replacement at Vonage's two-secret limit: when the account already holds two secrets, changing `secret` now revokes the old managed secret before creating the new one, instead of always failing with 403. With one secret the create-then-revoke order is kept. The old secret stops working immediately and, if the new create then fails, consumers are left without a working secret; the choice is made in `Diff`, which lists the account's secrets, so a preview and the apply can disagree if the count changes in between.
- BREAKING: `ApiSecret` import IDs without `ACCOUNT/SECRET_ID` are rejected (previously accepted and sent to Vonage with an empty account).
- `Application` refresh no longer drops a declared `messages.version` (Vonage does not return it per its OpenAPI definition and SDK types; unverified against a live account), which showed a spurious update on every `up` after a refresh.
- `Application` refresh now reads back a declared `publicKey`, so a key rotated outside Pulumi is detected and restored. The comparison uses the decoded PEM, so line endings, wrapping and trailing whitespace are not drift. That the application PUT accepts `keys.public_key` is unverified against a live account (only the test fake demonstrates it).
- `Number` buy that ends in an ambiguous error (transport error, 5xx; the purchase is never replayed) now re-checks ownership: if the account owns the number the ID is tracked with an init-failed error instead of leaving a billed number untracked. Definitive refusals (4xx, Vonage error codes) are returned as plain errors.
- `Application` updates preserve capabilities the provider does not model (`video`, `network_apis`, ...) instead of stripping them with the full-replace PUT.
- BREAKING (behaviour): `SubAccount` now reconciles `suspended` and `usePrimaryAccountBalance` against the Vonage defaults: removing `suspended: true` from the program unsuspends the subaccount, drift is visible after refresh even when the program omits the field, and import adopts non-default values. `SubAccount.Update` now always sends explicit `usePrimaryAccountBalance` and `suspended` values (the effective values, defaulting to true and false), so a subaccount whose suspension or balance mode was changed outside Pulumi is reset to the program on the next update.
- Removed a dead import silencer, added tests for the non-replay of billed POSTs, pagination, partial-create paths, import and drift for every resource, and made the test fake follow the documented API (no `messages.version` or `voiceStatusCallback` in GET responses).
- Docs: README now describes create-only `acquisition`, import ID formats, the rotation behaviour at the secret limit, and that `voiceStatusCallback` cannot be refreshed.
- Provider config environment-variable fallbacks now work: `apiKey`, `apiSecret`, `applicationId`, `privateKey`, `apiBaseUrl` and `restBaseUrl` fall back to `VONAGE_API_KEY`, `VONAGE_API_SECRET`, `VONAGE_APPLICATION_ID`, `VONAGE_PRIVATE_KEY`, `VONAGE_API_BASE_URL` and `VONAGE_REST_BASE_URL` when the config keys are unset. The fallbacks were previously declared only as `env` struct tags, which the infer framework ignores.

## 0.1.0

- Initial release: `Application`, `Number`, `SubAccount` and `ApiSecret` resources and `getApplication` and `getNumber` data sources for the Vonage account-management APIs.
- `SubAccount.secret` is create-only (the API cannot change it); later edits do not diff.
- `ApiSecret` replacement is decided by its `Diff` rather than schema-level `replaceOnChanges`, so importing an `ApiSecret` adopts the program's secret value in place instead of forcing a replacement (which would mint a new secret and revoke the old one). Changing the secret value or owning account of a managed secret replaces it; destroy treats an already-revoked secret as gone.
- `Number` voice routing is cleared on Vonage when removed from the program, and `voiceCallbackType/Value` and `voiceStatusCallback` are populated on refresh and import.
- The `apiBaseUrl` and `restBaseUrl` overrides must use `https` (loopback `http` is accepted for local fakes), since every request carries the account credentials as HTTP Basic auth.
- Docs: README notes the provider has not been validated against a live Vonage account.
