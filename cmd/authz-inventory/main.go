// Command authz-inventory generates GraphQL root-field inventory and checks policy coverage.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stashapp/stash/internal/authz"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

type inventory struct {
	SchemaVersion string          `json:"schema_version"`
	Surfaces      []authz.Surface `json:"surfaces"`
}

type schemaPatterns []string

func (p *schemaPatterns) String() string { return strings.Join(*p, ",") }
func (p *schemaPatterns) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("schema pattern is empty")
	}
	*p = append(*p, value)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "authz-inventory:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("authz-inventory", flag.ContinueOnError)
	var schemas schemaPatterns
	fs.Var(&schemas, "schema", "GraphQL schema path/glob; repeatable (defaults to gqlgen.yml source patterns)")
	outputPath := fs.String("output", "", "write generated inventory JSON")
	policyPath := fs.String("check-policy", "", "fail unless policy JSON exactly covers the schema inventory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(schemas) == 0 {
		schemas = schemaPatterns{"graphql/schema/types/*.graphql", "graphql/schema/*.graphql"}
	}
	surfaces, err := loadGraphQLInventory(schemas)
	if err != nil {
		return err
	}
	result := inventory{SchemaVersion: "1", Surfaces: surfaces}
	if *outputPath != "" {
		if err := writeInventory(*outputPath, result); err != nil {
			return err
		}
	}
	if *policyPath != "" {
		if err := checkPolicyCoverage(*policyPath, surfaces); err != nil {
			return err
		}
	}
	if *outputPath == "" && *policyPath == "" {
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	return nil
}

func loadGraphQLInventory(patterns []string) ([]authz.Surface, error) {
	var sources []*ast.Source
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("schema pattern matched no files: %s", pattern)
		}
		sort.Strings(matches)
		for _, path := range matches {
			if _, duplicate := seen[path]; duplicate {
				continue
			}
			seen[path] = struct{}{}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			sources = append(sources, &ast.Source{Name: path, Input: string(data)})
		}
	}
	schema, gqlErr := gqlparser.LoadSchema(sources...)
	if gqlErr != nil {
		return nil, gqlErr
	}
	var result []authz.Surface
	appendFields := func(definition *ast.Definition, kind authz.SurfaceKind) {
		if definition == nil {
			return
		}
		for _, field := range definition.Fields {
			if strings.HasPrefix(field.Name, "__") {
				continue
			}
			result = append(result, authz.Surface{Kind: kind, Name: field.Name})
		}
	}
	appendFields(schema.Query, authz.SurfaceGraphQLQuery)
	appendFields(schema.Mutation, authz.SurfaceGraphQLMutation)
	appendFields(schema.Subscription, authz.SurfaceGraphQLSubscription)
	sort.Slice(result, func(i, j int) bool { return result[i].Key() < result[j].Key() })
	return result, nil
}

func checkPolicyCoverage(path string, generated []authz.Surface) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var policy inventory
	decoderErr := json.Unmarshal(data, &policy)
	if decoderErr != nil {
		return decoderErr
	}
	if policy.SchemaVersion != "1" {
		return fmt.Errorf("unsupported policy schema version %q", policy.SchemaVersion)
	}
	registry, err := authz.NewRegistry(policy.Surfaces)
	if err != nil {
		return err
	}
	generatedKeys := make(map[string]struct{}, len(generated))
	for _, surface := range generated {
		generatedKeys[surface.Key()] = struct{}{}
	}
	var missing []string
	for key := range generatedKeys {
		parts := splitKey(key)
		if _, err := registry.Lookup(authz.SurfaceKind(parts[0]), parts[1]); err != nil {
			missing = append(missing, key)
		}
	}
	var unknown []string
	for _, surface := range registry.Surfaces() {
		if _, ok := generatedKeys[surface.Key()]; !ok {
			unknown = append(unknown, surface.Key())
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	if len(missing) > 0 || len(unknown) > 0 {
		return fmt.Errorf("policy drift: missing=%v unknown=%v", missing, unknown)
	}
	return nil
}

func splitKey(value string) [2]string {
	for i, char := range value {
		if char == ':' {
			return [2]string{value[:i], value[i+1:]}
		}
	}
	return [2]string{value, ""}
}

func writeInventory(path string, value inventory) error {
	if path == "" {
		return errors.New("output path is empty")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
