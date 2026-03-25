package integration

import (
	"testing"
)

func TestParsePermission(t *testing.T) {
	tests := []struct {
		input    string
		resource string
		action   string
		scopes   []string
		wantErr  bool
	}{
		{"issues.read:*", "issues", "read", []string{"*"}, false},
		{"issues.write:louismorgner/tiny-oc", "issues", "write", []string{"louismorgner/tiny-oc"}, false},
		{"pulls.read:*", "pulls", "read", []string{"*"}, false},
		{"pulls.comment:*", "pulls", "comment", []string{"*"}, false},
		{"issues.*:*", "issues", "*", []string{"*"}, false},
		{"send_message:*", "", "send_message", []string{"*"}, false},
		{"send_message:dm", "", "send_message", []string{"dm"}, false},
		{"read_messages:#eng,#ops", "", "read_messages", []string{"#eng", "#ops"}, false},
		{"issues.read:team/ENG", "issues", "read", []string{"team/ENG"}, false},

		// Error cases
		{"", "", "", nil, true},
		{"issues.read", "", "", nil, true},  // missing scope
		{":*", "", "", nil, true},           // missing resource.action
		{"issues.read:", "", "", nil, true},  // empty scope
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p, err := ParsePermission(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParsePermission(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePermission(%q) unexpected error: %v", tt.input, err)
			}
			if p.Resource != tt.resource {
				t.Errorf("resource = %q, want %q", p.Resource, tt.resource)
			}
			if p.Action != tt.action {
				t.Errorf("action = %q, want %q", p.Action, tt.action)
			}
			if len(p.Scopes) != len(tt.scopes) {
				t.Fatalf("scopes = %v, want %v", p.Scopes, tt.scopes)
			}
			for i, s := range p.Scopes {
				if s != tt.scopes[i] {
					t.Errorf("scopes[%d] = %q, want %q", i, s, tt.scopes[i])
				}
			}
		})
	}
}

func TestCheckPermission(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		action      string
		target      string
		want        bool
	}{
		{
			name:        "wildcard scope allows any target",
			permissions: []string{"issues.read:*"},
			action:      "issues.read",
			target:      "louismorgner/tiny-oc",
			want:        true,
		},
		{
			name:        "specific scope allows matching target",
			permissions: []string{"issues.write:louismorgner/tiny-oc"},
			action:      "issues.write",
			target:      "louismorgner/tiny-oc",
			want:        true,
		},
		{
			name:        "specific scope denies non-matching target",
			permissions: []string{"issues.write:louismorgner/tiny-oc"},
			action:      "issues.write",
			target:      "other/repo",
			want:        false,
		},
		{
			name:        "wildcard action allows any action on resource",
			permissions: []string{"issues.*:*"},
			action:      "issues.read",
			target:      "any-repo",
			want:        true,
		},
		{
			name:        "wildcard action matches write too",
			permissions: []string{"issues.*:*"},
			action:      "issues.write",
			target:      "any-repo",
			want:        true,
		},
		{
			name:        "default deny — no matching permission",
			permissions: []string{"issues.read:*"},
			action:      "pulls.read",
			target:      "any-repo",
			want:        false,
		},
		{
			name:        "action-only permission (no dot)",
			permissions: []string{"send_message:*"},
			action:      "send_message",
			target:      "#eng",
			want:        true,
		},
		{
			name:        "comma scope matches one of multiple",
			permissions: []string{"read_messages:#eng,#ops"},
			action:      "read_messages",
			target:      "#ops",
			want:        true,
		},
		{
			name:        "comma scope denies non-matching",
			permissions: []string{"read_messages:#eng,#ops"},
			action:      "read_messages",
			target:      "#general",
			want:        false,
		},
		{
			name:        "multiple permissions — first matches",
			permissions: []string{"issues.read:*", "pulls.read:*"},
			action:      "issues.read",
			target:      "any",
			want:        true,
		},
		{
			name:        "multiple permissions — second matches",
			permissions: []string{"issues.read:*", "pulls.read:*"},
			action:      "pulls.read",
			target:      "any",
			want:        true,
		},
		{
			name:        "empty permissions denies all",
			permissions: []string{},
			action:      "issues.read",
			target:      "any",
			want:        false,
		},
		{
			name:        "empty-resource permission does not match resource-prefixed action",
			permissions: []string{"send_message:*"},
			action:      "issues.send_message",
			target:      "any-repo",
			want:        false,
		},
		{
			name:        "resource-prefixed permission does not match action-only request",
			permissions: []string{"issues.read:*"},
			action:      "read",
			target:      "any",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perms, err := ParsePermissions(tt.permissions)
			if err != nil {
				t.Fatalf("ParsePermissions failed: %v", err)
			}
			got := CheckPermission(perms, tt.action, tt.target)
			if got != tt.want {
				t.Errorf("CheckPermission(%v, %q, %q) = %v, want %v",
					tt.permissions, tt.action, tt.target, got, tt.want)
			}
		})
	}
}

func TestFilterResponse(t *testing.T) {
	raw := map[string]interface{}{
		"ok":      true,
		"ts":      "12345",
		"channel": "C123",
		"secret":  "should-not-appear",
		"messages": []interface{}{
			map[string]interface{}{"text": "hello", "user": "U1", "internal_id": "x"},
			map[string]interface{}{"text": "world", "user": "U2", "internal_id": "y"},
		},
	}

	t.Run("whitelist top-level fields", func(t *testing.T) {
		filtered := filterResponse(raw, []string{"ok", "ts", "channel"})
		if _, ok := filtered["secret"]; ok {
			t.Error("secret field should not be in filtered response")
		}
		if filtered["ok"] != true {
			t.Error("ok field missing")
		}
		if filtered["ts"] != "12345" {
			t.Error("ts field missing")
		}
	})

	t.Run("whitelist array fields", func(t *testing.T) {
		filtered := filterResponse(raw, []string{"messages[].text", "messages[].user"})
		msgs, ok := filtered["messages"].(map[string]interface{})
		if !ok {
			t.Fatal("messages should be a map in filtered output")
		}
		texts, ok := msgs["text"].([]interface{})
		if !ok {
			t.Fatal("messages.text should be an array")
		}
		if len(texts) != 2 || texts[0] != "hello" || texts[1] != "world" {
			t.Errorf("messages.text = %v, want [hello, world]", texts)
		}
	})

	t.Run("no whitelist returns all", func(t *testing.T) {
		filtered := filterResponse(raw, nil)
		if _, ok := filtered["secret"]; !ok {
			t.Error("without whitelist, all fields should be returned")
		}
	})
}

func TestFilterArrayResponse(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{"number": 1.0, "title": "Bug", "state": "open", "secret": "x"},
		map[string]interface{}{"number": 2.0, "title": "Feature", "state": "closed", "secret": "y"},
	}

	t.Run("top-level array with [].field notation", func(t *testing.T) {
		result := filterArrayResponse(raw, []string{"[].number", "[].title", "[].state"})
		if len(result) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(result))
		}
		first, ok := result[0].(map[string]interface{})
		if !ok {
			t.Fatal("element should be a map")
		}
		if first["number"] != 1.0 {
			t.Errorf("number = %v, want 1", first["number"])
		}
		if first["title"] != "Bug" {
			t.Errorf("title = %v, want Bug", first["title"])
		}
		if _, ok := first["secret"]; ok {
			t.Error("secret field should be filtered out")
		}
	})

	t.Run("nested field in array elements", func(t *testing.T) {
		nested := []interface{}{
			map[string]interface{}{
				"number": 1.0,
				"user":   map[string]interface{}{"login": "alice", "id": 42.0},
			},
		}
		result := filterArrayResponse(nested, []string{"[].number", "[].user.login"})
		if len(result) != 1 {
			t.Fatalf("expected 1 element, got %d", len(result))
		}
		elem := result[0].(map[string]interface{})
		if elem["number"] != 1.0 {
			t.Errorf("number = %v, want 1", elem["number"])
		}
		user, ok := elem["user"].(map[string]interface{})
		if !ok {
			t.Fatal("user should be a map")
		}
		if user["login"] != "alice" {
			t.Errorf("user.login = %v, want alice", user["login"])
		}
	})

	t.Run("filterAnyResponse dispatches to array handler", func(t *testing.T) {
		result := filterAnyResponse(raw, []string{"[].number", "[].title"})
		arr, ok := result.([]interface{})
		if !ok {
			t.Fatal("filterAnyResponse should return []interface{} for array input")
		}
		if len(arr) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(arr))
		}
	})

	t.Run("filterAnyResponse with no whitelist returns raw", func(t *testing.T) {
		result := filterAnyResponse(raw, nil)
		arr, ok := result.([]interface{})
		if !ok {
			t.Fatal("should return original array")
		}
		if len(arr) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(arr))
		}
	})
}
