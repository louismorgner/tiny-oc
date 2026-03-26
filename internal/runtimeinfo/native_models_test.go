package runtimeinfo

import "testing"

func TestNativeProfiles(t *testing.T) {
	profiles := NativeProfiles()
	if len(profiles) == 0 {
		t.Fatal("NativeProfiles() returned no profiles")
	}
	if profiles[0].ID != "openai/gpt-4o-mini" {
		t.Fatalf("NativeProfiles()[0].ID = %q", profiles[0].ID)
	}
}

func TestResolveNativeProfileKnownModel(t *testing.T) {
	profile := ResolveNativeProfile("openai/gpt-4o-mini")
	if profile.Label != "GPT-4o Mini" {
		t.Fatalf("ResolveNativeProfile() = %#v", profile)
	}
	if !profile.SupportsTools || !profile.SupportsStreaming {
		t.Fatalf("ResolveNativeProfile() returned unsupported known profile: %#v", profile)
	}
}

func TestResolveNativeProfileUnknownModel(t *testing.T) {
	profile := ResolveNativeProfile("meta-llama/unknown")
	if profile.ID != "meta-llama/unknown" {
		t.Fatalf("ResolveNativeProfile().ID = %q", profile.ID)
	}
	if profile.Description != "custom OpenRouter model" {
		t.Fatalf("ResolveNativeProfile().Description = %q", profile.Description)
	}
}

func TestValidateNativeModel_RejectsUnknownWithoutOverride(t *testing.T) {
	err := ValidateNativeModel("meta-llama/unknown", false)
	if err == nil {
		t.Fatal("expected unknown native model to fail without override")
	}
}

func TestValidateNativeModel_AllowsUnknownWithOverride(t *testing.T) {
	if err := ValidateNativeModel("meta-llama/unknown", true); err != nil {
		t.Fatalf("expected override to allow unknown native model, got %v", err)
	}
}

func TestValidateNativeProfile_RequiresTools(t *testing.T) {
	err := ValidateNativeProfile(NativeModelProfile{
		ID:            "broken-model",
		SupportsTools: false,
	}, NativeBetaContract())
	if err == nil {
		t.Fatal("expected capability contract failure")
	}
}
