package run

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const sandboxModuleRoot = "../../../../.."

type wireField struct {
	omitempty bool
	nested    *wireStruct
}

type wireStruct struct {
	fields map[string]wireField
}

type contractProperty struct {
	Ref        string                      `yaml:"$ref"`
	Properties map[string]contractProperty `yaml:"properties"`
	Required   []string                    `yaml:"required"`
	Items      *contractProperty           `yaml:"items"`
	AllOf      []contractProperty          `yaml:"allOf"`
}

func resolved(p contractProperty, schemas map[string]contractProperty) contractProperty {
	if name, ok := strings.CutPrefix(p.Ref, "#/components/schemas/"); ok {
		return resolved(schemas[name], schemas)
	}
	if len(p.AllOf) == 0 {
		return p
	}
	merged := contractProperty{Properties: map[string]contractProperty{}, Required: p.Required}
	for name, prop := range p.Properties {
		merged.Properties[name] = prop
	}
	for _, part := range p.AllOf {
		r := resolved(part, schemas)
		for name, prop := range r.Properties {
			merged.Properties[name] = prop
		}
		merged.Required = append(merged.Required, r.Required...)
	}
	return merged
}

func readSandboxContract(t *testing.T) map[string]contractProperty {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(sandboxModuleRoot, "contracts", "openapi", "sandbox-provider.yaml"))
	if err != nil {
		t.Fatalf("reading the provider contract: %v", err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]contractProperty `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing the provider contract: %v", err)
	}
	return spec.Components.Schemas
}

func jsonTag(tag string) (name string, omitempty, onWire bool) {
	parts := strings.Split(tag, ",")
	if parts[0] == "" || parts[0] == "-" {
		return "", false, false
	}
	for _, p := range parts[1:] {
		omitempty = omitempty || p == "omitempty" || p == "omitzero"
	}
	return parts[0], omitempty, true
}

func reflectedWire(t reflect.Type) *wireStruct {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t.PkgPath() == "time" {
		return nil
	}
	w := &wireStruct{fields: map[string]wireField{}}
	for i := range t.NumField() {
		name, omitempty, ok := jsonTag(t.Field(i).Tag.Get("json"))
		if !ok {
			continue
		}
		w.fields[name] = wireField{omitempty: omitempty, nested: reflectedWire(t.Field(i).Type)}
	}
	return w
}

func sandboxStructs(t *testing.T) map[string]*ast.StructType {
	t.Helper()
	dir := filepath.Join(sandboxModuleRoot, "apps", "sandbox", "internal", "sandbox")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listing the sandbox provider: %v", err)
	}
	structs := map[string]*ast.StructType{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parsing the sandbox provider: %v", err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if spec, ok := n.(*ast.TypeSpec); ok {
				if st, ok := spec.Type.(*ast.StructType); ok {
					structs[spec.Name.Name] = st
				}
			}
			return true
		})
	}
	return structs
}

func parsedWire(st *ast.StructType, named map[string]*ast.StructType) *wireStruct {
	w := &wireStruct{fields: map[string]wireField{}}
	for _, f := range st.Fields.List {
		if f.Tag == nil {
			continue
		}
		tag, err := strconv.Unquote(f.Tag.Value)
		if err != nil {
			continue
		}
		name, omitempty, ok := jsonTag(reflect.StructTag(tag).Get("json"))
		if !ok {
			continue
		}
		w.fields[name] = wireField{omitempty: omitempty, nested: parsedFieldWire(f.Type, named)}
	}
	return w
}

func parsedFieldWire(expr ast.Expr, named map[string]*ast.StructType) *wireStruct {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return parsedFieldWire(e.X, named)
	case *ast.ArrayType:
		return parsedFieldWire(e.Elt, named)
	case *ast.StructType:
		return parsedWire(e, named)
	case *ast.Ident:
		if st, ok := named[e.Name]; ok {
			return parsedWire(st, named)
		}
	}
	return nil
}

func objectOf(p contractProperty, schemas map[string]contractProperty) *contractProperty {
	if p.Items != nil {
		return objectOf(*p.Items, schemas)
	}
	r := resolved(p, schemas)
	if len(r.Properties) == 0 {
		return nil
	}
	return &r
}

func wireProblems(path string, schema contractProperty, w *wireStruct, sender bool, schemas map[string]contractProperty) []string {
	var problems []string
	for name, field := range w.fields {
		property, declared := schema.Properties[name]
		if !declared {
			problems = append(problems, path+"."+name+": no such property in the contract")
			continue
		}
		if nested := objectOf(property, schemas); nested != nil && field.nested != nil {
			problems = append(problems, wireProblems(path+"."+name, *nested, field.nested, sender, schemas)...)
		}
	}
	for _, required := range schema.Required {
		field, present := w.fields[required]
		switch {
		case !present:
			problems = append(problems, path+"."+required+": required by the contract and no field carries it")
		case sender && field.omitempty:
			problems = append(problems, path+"."+required+": required by the contract but omitempty, so a zero value is left out")
		}
	}
	return problems
}

var sandboxWireSchemas = []struct {
	schema        string
	platform      reflect.Type
	platformSends bool
	sandbox       string
}{
	{"RunRequest", reflect.TypeOf(RunRequest{}), true, "RunRequest"},
	{"PackageRef", reflect.TypeOf(PackageRef{}), true, "PackageRef"},
	{"TestCaseSnapshotRef", reflect.TypeOf(TestCaseSnapshotRef{}), true, "TestCaseSnapshotRef"},
	{"RuntimeProfile", reflect.TypeOf(RuntimeProfile{}), true, "RuntimeProfile"},
	{"ResourceLimits", reflect.TypeOf(ResourceLimits{}), true, "ResourceLimits"},
	{"EgressPolicy", reflect.TypeOf(EgressPolicy{}), true, "EgressPolicy"},
	{"ObjectGrant", reflect.TypeOf(ObjectGrant{}), true, "ObjectGrant"},
	{"ModelGatewayGrant", reflect.TypeOf(ModelGatewayGrant{}), true, "ModelGatewayGrant"},
	{"TracePolicy", reflect.TypeOf(TracePolicy{}), true, "TracePolicy"},
	{"ProviderRun", reflect.TypeOf(ProviderRun{}), false, "ProviderRun"},
	{"ProviderRunList", reflect.TypeOf(ProviderRunList{}), false, "ProviderRunList"},
	{"RunResult", reflect.TypeOf(RunResult{}), false, "RunResult"},
	{"RunUsage", reflect.TypeOf(RunUsage{}), false, "RunUsage"},
	{"RunError", reflect.TypeOf(RunError{}), false, "RunError"},
	{"ProviderCapability", reflect.TypeOf(ProviderCapability{}), false, "ProviderCapability"},
}

func TestBothSidesOfTheSandboxContractCarryExactlyItsFields(t *testing.T) {
	schemas := readSandboxContract(t)
	named := sandboxStructs(t)
	var problems []string
	for _, s := range sandboxWireSchemas {
		schema, ok := schemas[s.schema]
		if !ok {
			problems = append(problems, s.schema+": the contract has no such schema")
			continue
		}
		schema = resolved(schema, schemas)
		problems = append(problems, wireProblems("platform "+s.schema, schema, reflectedWire(s.platform), s.platformSends, schemas)...)
		st, ok := named[s.sandbox]
		if !ok {
			problems = append(problems, "sandbox "+s.sandbox+": no such struct in the provider")
			continue
		}
		problems = append(problems, wireProblems("sandbox "+s.schema, schema, parsedWire(st, named), !s.platformSends, schemas)...)
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("the sandbox contract and its Go types disagree:\n  %s", strings.Join(problems, "\n  "))
	}
}
