// Binary noforceenew runs the noforceenew analyzer as a standalone go vet tool.
// Usage: go vet -vettool=$(which noforceenew) ./...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/zph/terraform-provider-mongodb/linters/noforceenew"
)

func main() {
	singlechecker.Main(noforceenew.Analyzer)
}
