# Vonage Provider Skills

Guidance for agents using the `vonage` Pulumi provider.

## When to use it
Declare Vonage applications, numbers, subaccounts and API secrets as code. Do not use it for sending messages or placing calls.

## Rules of thumb
1. Configure `vonage:apiKey` and secret `vonage:apiSecret`; never hardcode them.
2. Prefer `Number` with `acquisition: adopt` and `cancelOnDelete: false` for existing numbers. Use `buy` only on explicit instruction: it is billed.
3. Never set `cancelOnDelete: true` unless the user asked for destroy to release the number permanently.
4. `Application.privateKey` is a one-time secret output; export it as a secret or pass your own `publicKey`.
5. `Number.appId` is mutually exclusive with `moHttpUrl` and `voiceCallbackType`/`voiceCallbackValue`.
6. Subaccounts need primary account credentials and cannot be deleted; use `suspendOnDelete`.
7. Each account holds at most two API secrets. Rotate by adding the new `ApiSecret`, rolling consumers, then removing the old one. Secrets are 8-25 characters with a lowercase letter, an uppercase letter and a digit.
8. Use `getApplication` and `getNumber` to reference resources managed elsewhere.

## Resource and function tokens
`vonage:index:Application`, `vonage:index:Number`, `vonage:index:SubAccount`, `vonage:index:ApiSecret`, `vonage:index:getApplication`, `vonage:index:getNumber`.

## Roadmap
Number search-and-buy, account settings, balance transfers, JWT-authenticated APIs, Verify templates. See README.
