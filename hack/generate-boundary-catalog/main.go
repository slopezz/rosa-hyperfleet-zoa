// generate-boundary-catalog writes RC/MC Trusted Action catalogs for the boundary image.
package main

import (
	"fmt"
	"os"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/catalogdoc"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func main() {
	_ = actions.ListCatalog // ensure package links; inits register TAs
	dir := "boundary/catalog"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := catalogdoc.WriteBoundaryCatalogFiles(dir); err != nil {
		fmt.Fprintf(os.Stderr, "generate-boundary-catalog: %v\n", err)
		os.Exit(1)
	}
}
