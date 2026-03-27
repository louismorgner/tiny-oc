package runtime

import (
	"reflect"
	"testing"
)

func TestNativeToolNames(t *testing.T) {
	got := NativeToolNames()
	want := []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash", "Skill", "Question", "SubAgent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NativeToolNames() = %#v, want %#v", got, want)
	}
}

func TestNativeToolSetFiltersInRegistryOrder(t *testing.T) {
	got := nativeToolSet([]string{"Bash", "Read"})
	want := []string{"Read", "Bash"}
	if len(got) != len(want) {
		t.Fatalf("nativeToolSet() len = %d, want %d", len(got), len(want))
	}
	for i, spec := range got {
		if spec.Name != want[i] {
			t.Fatalf("nativeToolSet()[%d] = %q, want %q", i, spec.Name, want[i])
		}
	}
}
