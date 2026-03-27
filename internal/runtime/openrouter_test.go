package runtime

import "testing"

func TestNewNativeLLMClientFromEnv_PrefersTOCNativeBaseURL(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("TOC_NATIVE_BASE_URL", "http://localhost:8000")
	t.Setenv("OPENROUTER_BASE_URL", "http://localhost:9000")

	client, err := newNativeLLMClientFromEnv(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if client.baseURL != "http://localhost:8000" {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
}
