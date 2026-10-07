package utils

import (
	"fmt"
	"testing"

	"github.com/Laisky/go-utils/v6/json"
)

// TestFlattenMap verifies that FlattenMap rewrites a nested JSON-decoded map in place into "."-joined keys such
// as "b.c" and "b.d.e", keeps top-level scalar values unchanged, and drops keys whose value is an empty map.
func TestFlattenMap(t *testing.T) {
	data := map[string]any{}
	j := []byte(`{"a": "1", "b": {"c": 2, "d": {"e": 3}}, "f": 4, "g": {}}`)
	if err := json.Unmarshal(j, &data); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	FlattenMap(data, ".")
	if data["a"].(string) != "1" {
		t.Fatalf("expect %v, got %v", "1", data["a"])
	}
	if int(data["b.c"].(float64)) != 2 {
		t.Fatalf("expect %v, got %v", 2, data["b.c"])
	}
	if int(data["b.d.e"].(float64)) != 3 {
		t.Fatalf("expect %v, got %v", 3, data["b.d.e"])
	}
	if int(data["f"].(float64)) != 4 {
		t.Fatalf("expect %v, got %v", 4, data["f"])
	}
	if _, ok := data["g"]; ok {
		t.Fatalf("g should not exists")
	}
}

// ExampleFlattenMap demonstrates flattening a nested map in place with the "__" delimiter, producing keys such as
// "b__c" and "b__d__e".
func ExampleFlattenMap() {
	data := map[string]any{
		"a": "1",
		"b": map[string]any{
			"c": 2,
			"d": map[string]any{
				"e": 3,
			},
		},
	}
	FlattenMap(data, "__")
	fmt.Println(data)
	// Output: map[a:1 b__c:2 b__d__e:3]
}
