// Package noforceenew provides a go/analysis linter that detects ForceNew: true
// in Terraform schema.Schema composite literals. ForceNew causes destroy+recreate
// which is dangerous for database providers. DANGER-011
package noforceenew

import (
	"flag"
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const sdkSchemaPath = "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

// Analyzer reports ForceNew: true in Terraform schema.Schema literals.
var Analyzer = &analysis.Analyzer{
	Name:     "noforceenew",
	Doc:      "reports ForceNew: true in Terraform schema.Schema literals (DANGER-011)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

var allowFlag string

func init() {
	Analyzer.Flags.Init("noforceenew", flag.ExitOnError)
	Analyzer.Flags.StringVar(&allowFlag, "allow", "",
		"comma-separated file:field pairs to allow (e.g. resource_shard.go:shard_name)")
}

func parseAllowFlag() map[string]bool {
	allowed := make(map[string]bool)
	if allowFlag == "" {
		return allowed
	}
	for _, entry := range strings.Split(allowFlag, ",") {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			allowed[entry] = true
		}
	}
	return allowed
}

func run(pass *analysis.Pass) (interface{}, error) {
	allowed := parseAllowFlag()
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{(*ast.KeyValueExpr)(nil)}

	insp.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		kv := n.(*ast.KeyValueExpr)

		ident, ok := kv.Key.(*ast.Ident)
		if !ok || ident.Name != "ForceNew" {
			return true
		}
		boolLit, ok := kv.Value.(*ast.Ident)
		if !ok || boolLit.Name != "true" {
			return true
		}

		if !isInsideSchemaLit(pass, stack) {
			return true
		}

		fieldName := extractFieldName(stack)
		fileName := filepath.Base(pass.Fset.Position(kv.Pos()).Filename)

		key := fileName + ":" + fieldName
		if allowed[key] {
			return true
		}

		pass.Reportf(kv.Pos(),
			"ForceNew: true is banned (DANGER-010); use CustomizeDiff to block identity field changes instead")
		return true
	})
	return nil, nil
}

// isInsideSchemaLit checks whether the stack contains a composite literal
// whose type is schema.Schema from the Terraform plugin SDK.
func isInsideSchemaLit(pass *analysis.Pass, stack []ast.Node) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		lit, ok := stack[i].(*ast.CompositeLit)
		if !ok {
			continue
		}
		typ := pass.TypesInfo.TypeOf(lit)
		if typ == nil {
			continue
		}
		// Unwrap pointer
		if ptr, ok := typ.(*types.Pointer); ok {
			typ = ptr.Elem()
		}
		named, ok := typ.(*types.Named)
		if !ok {
			continue
		}
		obj := named.Obj()
		if obj.Pkg() != nil && obj.Pkg().Path() == sdkSchemaPath && obj.Name() == "Schema" {
			return true
		}
	}
	return false
}

// extractFieldName walks up the AST stack to find the Terraform field name.
// The pattern is: "field_name": { ForceNew: true } where field_name is a
// string key in the parent map[string]*schema.Schema composite literal.
func extractFieldName(stack []ast.Node) string {
	// Walk backwards looking for a KeyValueExpr whose Key is a BasicLit (string)
	// This represents the map entry like "parameter": { ... }
	for i := len(stack) - 1; i >= 0; i-- {
		kv, ok := stack[i].(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		lit, ok := kv.Key.(*ast.BasicLit)
		if !ok {
			continue
		}
		// Strip quotes from the string literal
		name := strings.Trim(lit.Value, `"`)
		if name != "" && name != "ForceNew" {
			return name
		}
	}
	return "<unknown>"
}
