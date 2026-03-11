// Minimal stub of the Terraform plugin SDK schema package for analysistest.
package schema

type ValueType int

const (
	TypeString ValueType = iota
	TypeBool
	TypeInt
	TypeFloat
	TypeList
	TypeMap
	TypeSet
)

type Schema struct {
	Type        ValueType
	Required    bool
	Optional    bool
	Computed    bool
	ForceNew    bool
	Sensitive   bool
	Default     interface{}
	Description string
	Elem        interface{}
}

type Resource struct {
	Schema map[string]*Schema
}
