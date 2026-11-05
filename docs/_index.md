---
title: Vonage Provider
meta_desc: Manage Vonage applications, numbers, subaccounts, API secrets and Verify v2 templates with Pulumi.
layout: package
---

An experimental Pulumi provider for [Vonage](https://developer.vonage.com) (formerly Nexmo), the communications APIs platform. It manages applications, virtual numbers, subaccounts, API secrets and Verify v2 templates as infrastructure.

## Installation

The Vonage provider is available as a package in all Pulumi-supported languages:

* JavaScript/TypeScript: [`@pulumi-labs/vonage`](https://www.npmjs.com/package/@pulumi-labs/vonage)
* Python: [`pulumi-labs-vonage`](https://pypi.org/project/pulumi-labs-vonage/)
* Go: [`github.com/pulumi-labs/pulumi-vonage/sdk/go/vonage`](https://github.com/pulumi-labs/pulumi-vonage)
* .NET: [`Pulumi.Labs.Vonage`](https://www.nuget.org/packages/Pulumi.Labs.Vonage)
* Java: [`com.pulumi.labs:vonage`](https://central.sonatype.com/artifact/com.pulumi.labs/vonage)

## Example Usage

{{< chooser language "typescript,python,yaml" >}}
{{% choosable language typescript %}}
```typescript
import * as vonage from "@pulumi-labs/vonage";

const app = new vonage.Application("app", {
    name: "my-app",
});

export const applicationId = app.applicationId;
```
{{% /choosable %}}
{{% choosable language python %}}
```python
import pulumi
import pulumi_labs_vonage as vonage

app = vonage.Application("app", name="my-app")

pulumi.export("applicationId", app.application_id)
```
{{% /choosable %}}
{{% choosable language yaml %}}
```yaml
resources:
  app:
    type: vonage:index:Application
    properties:
      name: my-app
outputs:
  applicationId: ${app.applicationId}
```
{{% /choosable %}}
{{< /chooser >}}

For runnable programs in every language, see the [`examples/`](https://github.com/pulumi-labs/pulumi-vonage/tree/main/examples) directory in the provider repository.

## Configuration

The provider works with no explicit `Provider` resource. Set your Vonage credentials as environment variables and every resource picks them up. See Installation & Configuration for the keys and their environment variables.
