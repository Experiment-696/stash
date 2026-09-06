package authz

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed graphql_policy.json
var graphqlPolicyJSON []byte

//go:embed http_policy.json
var httpPolicyJSON []byte

type policyDocument struct {
	SchemaVersion string    `json:"schema_version"`
	Surfaces      []Surface `json:"surfaces"`
}

// LoadGraphQLPolicy returns the reviewed, build-pinned GraphQL policy. Invalid
// policy is a startup error; callers must not substitute an empty registry.
func LoadGraphQLPolicy() (*Registry, error) {
	return loadPolicy("GraphQL", graphqlPolicyJSON)
}

func LoadHTTPPolicy() (*Registry, error) {
	return loadPolicy("HTTP", httpPolicyJSON)
}

func loadPolicy(label string, data []byte) (*Registry, error) {
	var document policyDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode embedded %s authorization policy: %w", label, err)
	}
	if document.SchemaVersion != "1" {
		return nil, fmt.Errorf("unsupported %s authorization policy schema version %q", label, document.SchemaVersion)
	}
	registry, err := NewRegistry(document.Surfaces)
	if err != nil {
		return nil, fmt.Errorf("validate embedded %s authorization policy: %w", label, err)
	}
	return registry, nil
}
