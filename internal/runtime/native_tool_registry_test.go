package runtime

import (
	"reflect"
	"strings"
	"testing"
)

func TestNativeToolNames(t *testing.T) {
	got := NativeToolNames()
	want := []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash", "Skill", "SubAgent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NativeToolNames() = %#v, want %#v", got, want)
	}
}

func TestNativeToolSetIncludesToolSearch(t *testing.T) {
	specs := nativeToolSet(nil)
	var found bool
	for _, spec := range specs {
		if spec.Name == "ToolSearch" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("nativeToolSet() should include ToolSearch meta-tool")
	}
}

func TestNativeToolSetFiltersInRegistryOrder(t *testing.T) {
	got := nativeToolSet([]string{"Bash", "Read"})
	// Should include Read, Bash, and ToolSearch (always included)
	var names []string
	for _, spec := range got {
		names = append(names, spec.Name)
	}
	if len(got) != 3 {
		t.Fatalf("nativeToolSet() names = %v, want [Read, Bash, ToolSearch]", names)
	}
	if got[0].Name != "Read" || got[1].Name != "Bash" || got[2].Name != "ToolSearch" {
		t.Fatalf("nativeToolSet() names = %v, want [Read, Bash, ToolSearch]", names)
	}
}

func TestNativeToolDefinitionsOmitsDeferredTools(t *testing.T) {
	specs := nativeToolSet(nil)
	defs := nativeToolDefinitions(specs)
	for _, def := range defs {
		// Deferred tools should not appear in the definitions
		switch def.Function.Name {
		case "Glob", "Grep", "Skill", "SubAgent":
			t.Fatalf("deferred tool %q should not appear in nativeToolDefinitions()", def.Function.Name)
		}
	}
	// Core tools should be present
	coreTools := map[string]bool{"Read": false, "Write": false, "Edit": false, "Bash": false, "ToolSearch": false}
	for _, def := range defs {
		coreTools[def.Function.Name] = true
	}
	for name, found := range coreTools {
		if !found {
			t.Fatalf("core tool %q missing from nativeToolDefinitions()", name)
		}
	}
}

func TestDeferredToolSummary(t *testing.T) {
	specs := nativeToolSet(nil)
	summary := DeferredToolSummary(specs)
	if summary == "" {
		t.Fatal("DeferredToolSummary() should not be empty")
	}
	if !strings.Contains(summary, "<deferred_tools>") {
		t.Fatal("DeferredToolSummary() should contain <deferred_tools> XML")
	}
	for _, name := range []string{"Glob", "Grep", "Skill", "SubAgent"} {
		if !strings.Contains(summary, name) {
			t.Fatalf("DeferredToolSummary() should mention %q", name)
		}
	}
}

func TestToolSearchHandler(t *testing.T) {
	specs := nativeToolSet(nil)
	// Find the ToolSearch spec
	var searchSpec *NativeToolSpec
	for i, spec := range specs {
		if spec.Name == "ToolSearch" {
			searchSpec = &specs[i]
			break
		}
	}
	if searchSpec == nil {
		t.Fatal("ToolSearch not found in tool set")
	}

	// Test fetching a deferred tool
	result := searchSpec.Handler(nativeToolContext{}, toolCall(t, "ToolSearch", map[string]interface{}{
		"tools": "Glob,Grep",
	}))
	if result.Step.Success == nil || !*result.Step.Success {
		t.Fatalf("ToolSearch failed: %s", result.Message)
	}
	if !strings.Contains(result.Message, "Glob") || !strings.Contains(result.Message, "Grep") {
		t.Fatalf("ToolSearch should return Glob and Grep schemas, got: %s", result.Message)
	}

	// Test fetching a non-existent tool
	result = searchSpec.Handler(nativeToolContext{}, toolCall(t, "ToolSearch", map[string]interface{}{
		"tools": "NonExistentTool",
	}))
	if result.Step.Success != nil && *result.Step.Success {
		t.Fatal("ToolSearch should fail for non-existent tool")
	}
}
