package runtimeinfo

import "fmt"

const DefaultRuntime = "claude-code"
const NativeRuntime = "toc-native"

type ModelOption struct {
	ID          string
	Label       string
	Description string
}

func Supported() []string {
	return []string{DefaultRuntime, NativeRuntime}
}

func ValidateRuntime(name string) error {
	switch name {
	case DefaultRuntime, NativeRuntime:
		return nil
	default:
		return fmt.Errorf("unknown runtime: %s", name)
	}
}

func DefaultModel(runtimeName string) string {
	switch runtimeName {
	case DefaultRuntime:
		return "sonnet"
	case NativeRuntime:
		profiles := NativeProfiles()
		if len(profiles) == 0 {
			return ""
		}
		return profiles[0].ID
	default:
		return ""
	}
}

func ModelOptions(runtimeName string) []ModelOption {
	switch runtimeName {
	case DefaultRuntime:
		return []ModelOption{
			{ID: "default", Label: "Default", Description: "Claude Code default for your account tier"},
			{ID: "sonnet", Label: "Sonnet", Description: "fast, great for most tasks"},
			{ID: "opus", Label: "Opus", Description: "most capable, deeper reasoning"},
			{ID: "haiku", Label: "Haiku", Description: "lightweight, quick responses"},
		}
	case NativeRuntime:
		profiles := NativeProfiles()
		options := make([]ModelOption, 0, len(profiles))
		for _, profile := range profiles {
			options = append(options, ModelOption{
				ID:          profile.ID,
				Label:       profile.Label,
				Description: profile.Description,
			})
		}
		return options
	default:
		return nil
	}
}

func ValidateModel(runtimeName, model string) error {
	return ValidateModelSelection(runtimeName, model, false)
}

func ValidateModelSelection(runtimeName, model string, allowCustomNativeModel bool) error {
	switch runtimeName {
	case DefaultRuntime:
		switch model {
		case "default", "sonnet", "opus", "haiku":
			return nil
		default:
			return fmt.Errorf("unknown model: %s (expected default, sonnet, opus, or haiku)", model)
		}
	case NativeRuntime:
		return ValidateNativeModel(model, allowCustomNativeModel)
	default:
		return fmt.Errorf("unknown runtime: %s", runtimeName)
	}
}
