# Changelog

## [Unreleased]

### Fixed

- Provider config environment-variable fallbacks now work: `apiKey`, `apiSecret`, `applicationId`, `privateKey`, `apiBaseUrl` and `restBaseUrl` fall back to `VONAGE_API_KEY`, `VONAGE_API_SECRET`, `VONAGE_APPLICATION_ID`, `VONAGE_PRIVATE_KEY`, `VONAGE_API_BASE_URL` and `VONAGE_REST_BASE_URL` when the config keys are unset. The fallbacks were previously declared only as `env` struct tags, which the infer framework ignores.

## 0.1.0

- Initial release: `Application`, `Number`, `SubAccount` and `ApiSecret` resources and `getApplication` and `getNumber` data sources for the Vonage account-management APIs.
- `SubAccount.secret` is create-only (the API cannot change it); later edits do not diff.
- `ApiSecret` replacement is decided by its `Diff` rather than schema-level `replaceOnChanges`, so importing an `ApiSecret` adopts the program's secret value in place instead of forcing a replacement (which would mint a new secret and revoke the old one). Changing the secret value or owning account of a managed secret replaces it; destroy treats an already-revoked secret as gone.
- `Number` voice routing is cleared on Vonage when removed from the program, and `voiceCallbackType/Value` and `voiceStatusCallback` are populated on refresh and import.
- The `apiBaseUrl` and `restBaseUrl` overrides must use `https` (loopback `http` is accepted for local fakes), since every request carries the account credentials as HTTP Basic auth.
- Docs: README notes the provider has not been validated against a live Vonage account.
