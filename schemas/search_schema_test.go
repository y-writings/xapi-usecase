package schemas_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestSearchSchemaAndExamples(t *testing.T) {
	document := readDocument(t)
	s := compile(t, document)
	examples := document["examples"].([]any)
	for i, example := range examples {
		if err := s.Validate(example); err != nil {
			t.Errorf("example %d: %v", i, err)
		}
	}
}

func TestSearchSchemaRejectsInvalidDocuments(t *testing.T) {
	s := compile(t, readDocument(t))
	complete := `{"status":"complete","search":{"query":"q","limit":1},` +
		`"retrieved_at":"now","post_count":0,"resource_count":0,"resources":[]}`
	incomplete := `{"status":"incomplete","search":{"query":"q","limit":1},` +
		`"retrieved_at":"now","post_count":0,"resource_count":0,"resources":[]}`
	cases := map[string]string{
		"missing required": `{"status":"complete"}`,
		"null array":       strings.Replace(complete, `"resources":[]`, `"resources":null`, 1),
		"bad enum":         strings.Replace(complete, `"complete"`, `"stopped"`, 1),
		"missing reason":   incomplete,
		"reason complete":  complete[:1] + `"incomplete_reason":"api_error",` + complete[1:],
		"unknown property": complete[:len(complete)-1] + `,"extra":true}`,
		"limit too high":   strings.Replace(complete, `"limit":1`, `"limit":1001`, 1),
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			var value any
			if err := json.Unmarshal([]byte(text), &value); err != nil {
				t.Fatal(err)
			}
			if err := s.Validate(value); err == nil {
				t.Fatal("invalid document was accepted")
			}
		})
	}
}

func readDocument(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile("search.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func compile(t *testing.T, doc any) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	if err := c.AddResource("search.schema.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("search.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
