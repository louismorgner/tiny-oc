package runtimeinfo

import (
	"fmt"
	"strings"
)

type NativeModelProfile struct {
	ID                string
	Label             string
	Description       string
	SupportsTools     bool
	SupportsStreaming bool
}

type NativeCapabilityContract struct {
	RequiresTools bool
}

var nativeModelProfiles = []NativeModelProfile{
	{
		ID:                "openai/gpt-4o-mini",
		Label:             "GPT-4o Mini",
		Description:       "fast default for agent-first runtime iteration",
		SupportsTools:     true,
		SupportsStreaming: true,
	},
	{
		ID:                "openai/gpt-4o",
		Label:             "GPT-4o",
		Description:       "stronger OpenAI general-purpose model",
		SupportsTools:     true,
		SupportsStreaming: true,
	},
	{
		ID:                "anthropic/claude-sonnet-4",
		Label:             "Claude Sonnet 4",
		Description:       "strong coding and reasoning via OpenRouter",
		SupportsTools:     true,
		SupportsStreaming: true,
	},
	{
		ID:                "openai/gpt-5.4",
		Label:             "GPT-5.4",
		Description:       "latest OpenAI flagship model with 1M+ context",
		SupportsTools:     true,
		SupportsStreaming: true,
	},
	{
		ID:                "openai/gpt-5.3-codex",
		Label:             "GPT-5.3 Codex",
		Description:       "OpenAI code-specialized model with 400k context",
		SupportsTools:     true,
		SupportsStreaming: true,
	},
}

func NativeProfiles() []NativeModelProfile {
	out := make([]NativeModelProfile, len(nativeModelProfiles))
	copy(out, nativeModelProfiles)
	return out
}

func NativeBetaContract() NativeCapabilityContract {
	return NativeCapabilityContract{
		RequiresTools: true,
	}
}

func LookupNativeProfile(model string) (NativeModelProfile, bool) {
	for _, profile := range nativeModelProfiles {
		if profile.ID == model {
			return profile, true
		}
	}
	return NativeModelProfile{}, false
}

func SupportedNativeModelIDs() []string {
	ids := make([]string, 0, len(nativeModelProfiles))
	for _, profile := range nativeModelProfiles {
		ids = append(ids, profile.ID)
	}
	return ids
}

func ValidateNativeProfile(profile NativeModelProfile, contract NativeCapabilityContract) error {
	if contract.RequiresTools && !profile.SupportsTools {
		return fmt.Errorf("missing required tool-calling support")
	}
	return nil
}

func ValidateNativeModel(model string, allowCustom bool) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("missing model")
	}

	profile, ok := LookupNativeProfile(model)
	if !ok {
		if allowCustom {
			return nil
		}
		return fmt.Errorf("native model %q is not in the supported beta profile set; use a supported model or set allow_custom_native_model: true", model)
	}

	if err := ValidateNativeProfile(profile, NativeBetaContract()); err != nil {
		return fmt.Errorf("native model %q does not satisfy the toc-native beta capability contract: %w", model, err)
	}
	return nil
}

func ResolveNativeProfile(model string) NativeModelProfile {
	if profile, ok := LookupNativeProfile(model); ok {
		return profile
	}
	return NativeModelProfile{
		ID:                model,
		Label:             model,
		Description:       "custom OpenRouter model",
		SupportsTools:     true,
		SupportsStreaming: true,
	}
}
