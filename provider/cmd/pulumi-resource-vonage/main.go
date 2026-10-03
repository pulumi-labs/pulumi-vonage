// Command pulumi-resource-vonage is the entry point for the vonage Pulumi provider binary.
// It is invoked by the Pulumi engine and serves the provider gRPC protocol over stdin/stdout.
package main

import (
	"context"
	"os"

	"github.com/pulumi/autoregistry/lib/providers/vonage/provider/pkg/provider"
	"github.com/pulumi/autoregistry/lib/providers/vonage/provider/pkg/version"
	pgp "github.com/pulumi/pulumi-go-provider"
)

func main() {
	prov, err := provider.New()
	if err != nil {
		_, _ = os.Stderr.WriteString("failed to construct provider: " + err.Error() + "\n")
		os.Exit(1)
	}
	if err := pgp.RunProvider(context.Background(), "vonage", version.Version, prov); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
