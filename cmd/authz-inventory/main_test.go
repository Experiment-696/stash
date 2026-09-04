package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/authz"
)

func TestLoadGraphQLInventory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.graphql")
	schema := "type Query { b: String a: String }\ntype Mutation { update: Boolean }\ntype Subscription { changed: Boolean }\n"
	if err := os.WriteFile(path, []byte(schema), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadGraphQLInventory([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0].Kind != authz.SurfaceGraphQLMutation || got[1].Name != "a" {
		t.Fatalf("unexpected inventory: %#v", got)
	}
}

func TestLoadGraphQLInventoryAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "root.graphql"), []byte("type Query { lookup(input: LookupInput!): String }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "types.graphql"), []byte("input LookupInput { value: String! }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadGraphQLInventory([]string{filepath.Join(dir, "*.graphql")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "lookup" {
		t.Fatalf("unexpected multi-file inventory: %#v", got)
	}
}

func TestLoadGraphQLInventoryFiltersIntrospection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.graphql")
	if err := os.WriteFile(path, []byte("type Query { visible: String }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadGraphQLInventory([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || strings.HasPrefix(got[0].Name, "__") {
		t.Fatalf("introspection field leaked into inventory: %#v", got)
	}
}

func TestPolicyCoverageFailsOnDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	policy := `{"schema_version":"1","surfaces":[{"kind":"GRAPHQL_QUERY","name":"known","capability":"library.read","owner_scoped":false}]}`
	if err := os.WriteFile(path, []byte(policy), 0o600); err != nil {
		t.Fatal(err)
	}
	generated := []authz.Surface{{Kind: authz.SurfaceGraphQLQuery, Name: "newField"}}
	err := checkPolicyCoverage(path, generated)
	if err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("drift was not reported completely: %v", err)
	}
}

func TestWriteInventoryRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	if err := writeInventory(path, inventory{SchemaVersion: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := writeInventory(path, inventory{SchemaVersion: "1"}); err == nil {
		t.Fatal("inventory overwrite unexpectedly allowed")
	}
}
