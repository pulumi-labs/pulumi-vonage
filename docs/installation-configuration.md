---
title: Vonage Installation & Configuration
meta_desc: How to install the Pulumi Vonage provider and its language SDKs, and how to configure the account credentials.
layout: package
---

## Installation

The Vonage provider is available as a package in all Pulumi-supported languages:

* JavaScript/TypeScript: [`@pulumi-labs/vonage`](https://www.npmjs.com/package/@pulumi-labs/vonage)
* Python: [`pulumi-labs-vonage`](https://pypi.org/project/pulumi-labs-vonage/)
* Go: [`github.com/pulumi-labs/pulumi-vonage/sdk/go/vonage`](https://github.com/pulumi-labs/pulumi-vonage)
* .NET: [`Pulumi.Labs.Vonage`](https://www.nuget.org/packages/Pulumi.Labs.Vonage)
* Java: [`com.pulumi.labs:vonage`](https://central.sonatype.com/artifact/com.pulumi.labs/vonage)

In normal usage, installing one of the language SDKs above is enough. The first `pulumi up` automatically downloads the matching provider binary from the URL embedded in the SDK, so no separate step is required.

### Provider Binary (optional)

If you need to install the provider binary explicitly (for example, in an air-gapped environment or to prewarm a CI cache), use the `pulumi plugin` command:

```bash
pulumi plugin install resource vonage <version> --server github://api.github.com/pulumi-labs/pulumi-vonage
```

Replace `<version>` with your desired release (see the [releases page](https://github.com/pulumi-labs/pulumi-vonage/releases)).

## Configuration

The provider needs the API key and API secret of your Vonage account. It needs no explicit `Provider` resource and no stack config when they are available as environment variables:

```bash
export VONAGE_API_KEY=<api key>
export VONAGE_API_SECRET=<api secret>
```

You can also set them as provider configuration:

```bash
pulumi config set vonage:apiKey <api key>
pulumi config set --secret vonage:apiSecret <api secret>
```

Explicit Pulumi config wins over the environment variable. The following keys are available:

| Key | Env var | Default | Description |
|---|---|---|---|
| `apiKey` | `VONAGE_API_KEY` | none | The Vonage account API key. |
| `apiSecret` (secret) | `VONAGE_API_SECRET` | none | The Vonage account API secret. |
| `applicationId` | `VONAGE_APPLICATION_ID` | none | Application ID the JWT is signed for. Required by `User`, `getUser` and `getUsers`; not used by the other resources. |
| `privateKey` (secret) | `VONAGE_PRIVATE_KEY` | none | PEM private key for `applicationId` (PKCS#8 or PKCS#1). Required by `User`, `getUser` and `getUsers`; not used by the other resources. |
| `apiBaseUrl` | `VONAGE_API_BASE_URL` | `https://api.nexmo.com` | Override for the Application, Subaccounts and Secrets API host. For tests. |
| `restBaseUrl` | `VONAGE_REST_BASE_URL` | `https://rest.nexmo.com` | Override for the Numbers API host. For tests. |

An explicit `vonage` provider resource is only needed to target several accounts in one program.
