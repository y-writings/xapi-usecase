package schemas_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestUserPostsSchemaExamples(t *testing.T) {
	b, err := os.ReadFile("user-posts.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(b, &document); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("user-posts.schema.json", document); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("user-posts.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for i, example := range document["examples"].([]any) {
		if err := s.Validate(example); err != nil {
			t.Errorf("example %d: %v", i, err)
		}
	}
}
