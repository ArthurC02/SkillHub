package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

var limitKeywords = []struct{ key, suffix string }{
	{"maxLength", "MaxLength"},
	{"maxItems", "MaxItems"},
	{"x-max-bytes", "MaxBytes"},
}

var httpMethods = []string{"get", "put", "post", "delete", "patch"}

type limits map[string]int

func main() {
	contract := flag.String("contract", "", "OpenAPI document to read")
	goOut := flag.String("go", "", "Go file to write")
	tsOut := flag.String("ts", "", "TypeScript file to write")
	flag.Parse()
	if err := run(*contract, *goOut, *tsOut); err != nil {
		fmt.Fprintln(os.Stderr, "contractlimits:", err)
		os.Exit(1)
	}
}

func run(contract, goOut, tsOut string) error {
	data, err := os.ReadFile(contract)
	if err != nil {
		return err
	}
	found, err := collect(data)
	if err != nil {
		return err
	}
	goCode, err := goSource(found)
	if err != nil {
		return err
	}
	if err := os.WriteFile(goOut, goCode, 0o644); err != nil {
		return err
	}
	return os.WriteFile(tsOut, []byte(tsSource(found)), 0o644)
}

func collect(data []byte) (limits, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("empty document")
	}
	root := doc.Content[0]
	found := limits{}
	for name, schema := range pairs(child(child(root, "components"), "schemas")) {
		if err := walkSchema(found, pascal(name), schema); err != nil {
			return nil, err
		}
	}
	for _, path := range pairs(child(root, "paths")) {
		for _, method := range httpMethods {
			if err := walkOperation(found, child(path, method)); err != nil {
				return nil, err
			}
		}
	}
	return found, nil
}

func walkOperation(found limits, op *yaml.Node) error {
	if op == nil {
		return nil
	}
	id := child(op, "operationId")
	if id == nil {
		return fmt.Errorf("an operation at line %d has no operationId", op.Line)
	}
	prefix := pascal(id.Value)
	for _, param := range sequence(child(op, "parameters")) {
		if name := child(param, "name"); name != nil {
			if err := walkSchema(found, prefix+pascal(name.Value), child(param, "schema")); err != nil {
				return err
			}
		}
	}
	for _, media := range pairs(child(child(op, "requestBody"), "content")) {
		if err := walkSchema(found, prefix+"Body", child(media, "schema")); err != nil {
			return err
		}
	}
	return nil
}

func walkSchema(found limits, prefix string, schema *yaml.Node) error {
	if schema == nil || schema.Kind != yaml.MappingNode {
		return nil
	}
	for _, keyword := range limitKeywords {
		if value := child(schema, keyword.key); value != nil {
			if err := record(found, prefix+keyword.suffix, value); err != nil {
				return err
			}
		}
	}
	for _, sub := range subschemas(prefix, schema) {
		if err := walkSchema(found, sub.prefix, sub.schema); err != nil {
			return err
		}
	}
	return nil
}

type namedSchema struct {
	prefix string
	schema *yaml.Node
}

func subschemas(prefix string, schema *yaml.Node) []namedSchema {
	subs := []namedSchema{{prefix + "Item", child(schema, "items")}}
	for name, property := range pairs(child(schema, "properties")) {
		subs = append(subs, namedSchema{prefix + pascal(name), property})
	}
	for _, combinator := range []string{"allOf", "oneOf", "anyOf"} {
		for _, member := range sequence(child(schema, combinator)) {
			subs = append(subs, namedSchema{prefix, member})
		}
	}
	return subs
}

func record(found limits, name string, value *yaml.Node) error {
	n, err := strconv.Atoi(value.Value)
	if err != nil {
		return fmt.Errorf("%s at line %d is not an integer: %q", name, value.Line, value.Value)
	}
	if previous, seen := found[name]; seen && previous != n {
		return fmt.Errorf("%s is %d at line %d but %d elsewhere; two limits share one name", name, n, value.Line, previous)
	}
	found[name] = n
	return nil
}

func child(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func pairs(node *yaml.Node) map[string]*yaml.Node {
	out := map[string]*yaml.Node{}
	if node == nil || node.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		out[node.Content[i].Value] = node.Content[i+1]
	}
	return out
}

func sequence(node *yaml.Node) []*yaml.Node {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	return node.Content
}

func pascal(name string) string {
	var b strings.Builder
	upper := true
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			upper = true
			continue
		}
		if upper {
			r = unicode.ToUpper(r)
			upper = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func sortedNames(found limits) []string {
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func goSource(found limits) ([]byte, error) {
	var b strings.Builder
	b.WriteString("// Code generated by apps/platform/cmd/contractlimits from contracts/openapi/public.yaml. DO NOT EDIT.\n\npackage gen\n\nconst (\n")
	for _, name := range sortedNames(found) {
		fmt.Fprintf(&b, "\t%s = %d\n", name, found[name])
	}
	b.WriteString(")\n")
	return format.Source([]byte(b.String()))
}

func tsSource(found limits) string {
	var b strings.Builder
	b.WriteString("// Code generated by apps/platform/cmd/contractlimits from contracts/openapi/public.yaml. DO NOT EDIT.\n\n")
	for _, name := range sortedNames(found) {
		fmt.Fprintf(&b, "export const %s = %d;\n", name, found[name])
	}
	return b.String()
}
