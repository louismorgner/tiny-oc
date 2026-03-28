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
	ContextWindow     int // total context window in tokens (0 = unknown)
	MaxOutputTokens   int // max output tokens (0 = use provider default)
	ReservedBuffer    int // tokens reserved for tool defs, system overhead
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
		ContextWindow:     128000,
		MaxOutputTokens:   16384,
		ReservedBuffer:    4096,
	},
	{
		ID:                "openai/gpt-4o",
		Label:             "GPT-4o",
		Description:       "stronger OpenAI general-purpose model",
		SupportsTools:     true,
		SupportsStreaming: true,
		ContextWindow:     128000,
		MaxOutputTokens:   16384,
		ReservedBuffer:    4096,
	},
	{
		ID:                "anthropic/claude-sonnet-4",
		Label:             "Claude Sonnet 4",
		Description:       "strong coding and reasoning via OpenRouter",
		SupportsTools:     true,
		SupportsStreaming: true,
		ContextWindow:     200000,
		MaxOutputTokens:   16384,
		ReservedBuffer:    4096,
	},
	{
		ID:                "openai/gpt-5.4",
		Label:             "GPT-5.4",
		Description:       "latest OpenAI flagship model with 1M+ context",
		SupportsTools:     true,
		SupportsStreaming: true,
		ContextWindow:     1048576,
		MaxOutputTokens:   32768,
		ReservedBuffer:    8192,
	},
	{
		ID:                "openai/gpt-5.3-codex",
		Label:             "GPT-5.3 Codex",
		Description:       "OpenAI code-specialized model with 400k context",
		SupportsTools:     true,
		SupportsStreaming: true,
		ContextWindow:     400000,
		MaxOutputTokens:   32768,
		ReservedBuffer:    8192,
	},
	{
		ID:                "anthropic/claude-sonnet-4.6",
		Label:             "Claude Sonnet 4.6",
		Description:       "Anthropic mid-tier model with strong coding and reasoning at lower cost",
		SupportsTools:     true,
		SupportsStreaming: true,
		ContextWindow:     200000,
		MaxOutputTokens:   16384,
		ReservedBuffer:    4096,
	},
	{
		ID:                "anthropic/claude-opus-4.6",
		Label:             "Claude Opus 4.6",
		Description:       "Anthropic flagship model for coding and agentic workflows with 1M context",
		SupportsTools:     true,
		SupportsStreaming: true,
		ContextWindow:     1000000,
		MaxOutputTokens:   128000,
		ReservedBuffer:    8192,
	},
	{
		ID:                "z-ai/glm-5",
		Label:             "GLM-5",
		Description:       "Zhipu AI flagship model with strong multilingual and tool-use capabilities",
		SupportsTools:     true,
		SupportsStreaming: true,
		ContextWindow:     80000,
		MaxOutputTokens:   8192,
		ReservedBuffer:    4096,
	},
	{
		ID:                "moonshotai/kimi-k2.5",
		Label:             "Kimi K2.5",
		Description:       "Moonshot long-context model with 262k context and strong reasoning",
		SupportsTools:     true,
		SupportsStreaming: true,
		ContextWindow:     262144,
		MaxOutputTokens:   65535,
		ReservedBuffer:    8192,
	},
	{
		ID:                "xiaomi/mimo-v2-flash",
		Label:             "MiMo-V2-Flash",
		Description:       "Xiaomi MoE reasoning model with 309B params and fast inference",
		SupportsTools:     true,
		SupportsStreaming: true,
		ContextWindow:     262144,
		MaxOutputTokens:   65536,
		ReservedBuffer:    8192,
	},
	{
		ID:                "qwen/qwen3.5-397b-a17b",
		Label:             "Qwen 3.5 397B",
		Description:       "Alibaba large MoE model with 397B total params and 17B active",
		SupportsTools:     true,
		SupportsStreaming: true,
		ContextWindow:     262144,
		MaxOutputTokens:   65536,
		ReservedBuffer:    8192,
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
		ContextWindow:     128000, // conservative default for unknown models
		MaxOutputTokens:   8192,
		ReservedBuffer:    4096,
	}
}
