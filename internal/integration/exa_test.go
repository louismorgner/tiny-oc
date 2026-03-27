package integration

import (
	"reflect"
	"testing"
)

func TestBuildExaRequestBody_Search(t *testing.T) {
	params := map[string]string{
		"query":      "best Go frameworks",
		"numResults": "10",
		"type":       "auto",
	}

	body := BuildExaRequestBody("search", params)

	// query should be a string
	if body["query"] != "best Go frameworks" {
		t.Errorf("expected query to be 'best Go frameworks', got: %v", body["query"])
	}

	// numResults should be coerced to int
	if body["numResults"] != 10 {
		t.Errorf("expected numResults to be int 10, got: %v (%T)", body["numResults"], body["numResults"])
	}

	// type should be a string
	if body["type"] != "auto" {
		t.Errorf("expected type to be 'auto', got: %v", body["type"])
	}

	// contents.highlights should be injected by default
	contents, ok := body["contents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected contents to be a map, got: %T", body["contents"])
	}
	if contents["highlights"] != true {
		t.Errorf("expected contents.highlights to be true, got: %v", contents["highlights"])
	}
}

func TestBuildExaRequestBody_ArrayParams(t *testing.T) {
	params := map[string]string{
		"query":          "AI research",
		"includeDomains": "arxiv.org, scholar.google.com",
		"excludeDomains": "reddit.com",
	}

	body := BuildExaRequestBody("search", params)

	include, ok := body["includeDomains"].([]string)
	if !ok {
		t.Fatalf("expected includeDomains to be []string, got: %T", body["includeDomains"])
	}
	expected := []string{"arxiv.org", "scholar.google.com"}
	if !reflect.DeepEqual(include, expected) {
		t.Errorf("expected includeDomains %v, got: %v", expected, include)
	}

	exclude, ok := body["excludeDomains"].([]string)
	if !ok {
		t.Fatalf("expected excludeDomains to be []string, got: %T", body["excludeDomains"])
	}
	if !reflect.DeepEqual(exclude, []string{"reddit.com"}) {
		t.Errorf("expected excludeDomains [reddit.com], got: %v", exclude)
	}
}

func TestBuildExaRequestBody_FindSimilar(t *testing.T) {
	params := map[string]string{
		"url":        "https://example.com/article",
		"numResults": "3",
	}

	body := BuildExaRequestBody("find_similar", params)

	if body["url"] != "https://example.com/article" {
		t.Errorf("expected url, got: %v", body["url"])
	}
	if body["numResults"] != 3 {
		t.Errorf("expected numResults 3, got: %v", body["numResults"])
	}

	// find_similar should also get default highlights
	contents, ok := body["contents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected contents map, got: %T", body["contents"])
	}
	if contents["highlights"] != true {
		t.Errorf("expected contents.highlights true")
	}
}

func TestBuildExaRequestBody_GetContents(t *testing.T) {
	params := map[string]string{
		"ids": "https://example.com/a, https://example.com/b",
	}

	body := BuildExaRequestBody("get_contents", params)

	ids, ok := body["ids"].([]string)
	if !ok {
		t.Fatalf("expected ids to be []string, got: %T", body["ids"])
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 ids, got: %d", len(ids))
	}

	// get_contents should inject contents.text by default
	contents, ok := body["contents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected contents map, got: %T", body["contents"])
	}
	if contents["text"] != true {
		t.Errorf("expected contents.text true")
	}
}

func TestBuildExaRequestBody_Extract(t *testing.T) {
	params := map[string]string{
		"ids":    "https://example.com/page1, https://example.com/page2",
		"schema": `{"type":"object","properties":{"title":{"type":"string"},"price":{"type":"number"}},"required":["title"]}`,
		"query":  "product details",
	}

	body := BuildExaRequestBody("extract", params)

	// ids should be split into array
	ids, ok := body["ids"].([]string)
	if !ok {
		t.Fatalf("expected ids to be []string, got: %T", body["ids"])
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 ids, got: %d", len(ids))
	}

	// schema and query should be removed from top level
	if _, ok := body["schema"]; ok {
		t.Error("schema should not be at top level")
	}
	if _, ok := body["query"]; ok {
		t.Error("query should not be at top level")
	}

	// contents.summary should contain the schema and query
	contents, ok := body["contents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected contents to be a map, got: %T", body["contents"])
	}
	summary, ok := contents["summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected contents.summary to be a map, got: %T", contents["summary"])
	}

	// schema should be parsed JSON, not a string
	schema, ok := summary["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema to be parsed JSON object, got: %T", summary["schema"])
	}
	if schema["type"] != "object" {
		t.Errorf("expected schema.type to be 'object', got: %v", schema["type"])
	}

	// query should be passed through
	if summary["query"] != "product details" {
		t.Errorf("expected summary.query to be 'product details', got: %v", summary["query"])
	}
}

func TestBuildExaRequestBody_ExtractNoQuery(t *testing.T) {
	params := map[string]string{
		"ids":    "https://example.com/page1",
		"schema": `{"type":"object","properties":{"name":{"type":"string"}}}`,
	}

	body := BuildExaRequestBody("extract", params)

	contents, ok := body["contents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected contents map, got: %T", body["contents"])
	}
	summary, ok := contents["summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected summary map, got: %T", contents["summary"])
	}

	// query should not be present
	if _, ok := summary["query"]; ok {
		t.Error("expected no query in summary when not provided")
	}

	// schema should still be there
	if _, ok := summary["schema"]; !ok {
		t.Error("expected schema in summary")
	}
}

func TestBuildExaRequestBody_InvalidNumResults(t *testing.T) {
	params := map[string]string{
		"query":      "test",
		"numResults": "not-a-number",
	}

	body := BuildExaRequestBody("search", params)

	// Should fall back to string when parsing fails
	if body["numResults"] != "not-a-number" {
		t.Errorf("expected numResults to be string fallback, got: %v", body["numResults"])
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b, c", []string{"a", "b", "c"}},
		{"single", []string{"single"}},
		{"a,,b", []string{"a", "b"}},
		{" a , b ", []string{"a", "b"}},
	}

	for _, tt := range tests {
		result := splitCSV(tt.input)
		if !reflect.DeepEqual(result, tt.expected) {
			t.Errorf("splitCSV(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}
