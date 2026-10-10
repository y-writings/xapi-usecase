package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func validateSearchJSON(t *testing.T, data []byte) {
	t.Helper()
	schemaData, err := os.ReadFile("../../schemas/search.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schemaDocument, outputDocument any
	if err := json.Unmarshal(schemaData, &schemaDocument); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &outputDocument); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("search.schema.json", schemaDocument); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("search.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(outputDocument); err != nil {
		t.Fatalf("search output does not match schema: %v", err)
	}
}
