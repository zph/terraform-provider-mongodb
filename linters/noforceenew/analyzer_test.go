package noforceenew_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/zph/terraform-provider-mongodb/linters/noforceenew"
)

func TestNoForceNew(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, noforceenew.Analyzer, "bad", "good")
}
