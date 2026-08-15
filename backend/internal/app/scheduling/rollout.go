package scheduling

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type RolloutMode string

const (
	RolloutOff     RolloutMode = "off"
	RolloutShadow  RolloutMode = "shadow"
	RolloutEnforce RolloutMode = "enforce"
)

type RolloutConfig struct {
	Default   RolloutMode                       `json:"default"`
	Platforms map[string]RolloutMode            `json:"platforms,omitempty"`
	Groups    map[string]map[string]RolloutMode `json:"groups,omitempty"`
}

func ParseRolloutConfig(raw string) RolloutConfig {
	config := RolloutConfig{Default: RolloutOff}
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &config) != nil {
		return RolloutConfig{Default: RolloutOff}
	}
	config.Default = normalizeRolloutMode(config.Default)
	return config
}

func (c RolloutConfig) Resolve(platform string, groupID *int64) RolloutMode {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if groupID != nil {
		if groups := c.Groups[platform]; groups != nil {
			if mode, ok := groups[strings.TrimSpace(formatRolloutGroupID(*groupID))]; ok {
				return normalizeRolloutMode(mode)
			}
		}
	}
	if mode, ok := c.Platforms[platform]; ok {
		return normalizeRolloutMode(mode)
	}
	return normalizeRolloutMode(c.Default)
}

func (c RolloutConfig) Clone() RolloutConfig {
	result := RolloutConfig{Default: c.Default}
	if c.Platforms != nil {
		result.Platforms = make(map[string]RolloutMode, len(c.Platforms))
		for platform, mode := range c.Platforms {
			result.Platforms[platform] = mode
		}
	}
	if c.Groups != nil {
		result.Groups = make(map[string]map[string]RolloutMode, len(c.Groups))
		for platform, groups := range c.Groups {
			clonedGroups := make(map[string]RolloutMode, len(groups))
			for groupID, mode := range groups {
				clonedGroups[groupID] = mode
			}
			result.Groups[platform] = clonedGroups
		}
	}
	return result
}

func (c RolloutConfig) Validate() error {
	if !validRolloutMode(c.Default) {
		return fmt.Errorf("invalid default rollout mode %q", c.Default)
	}
	for platform, mode := range c.Platforms {
		if strings.TrimSpace(platform) == "" {
			return fmt.Errorf("rollout platform must not be empty")
		}
		if !validRolloutMode(mode) {
			return fmt.Errorf("invalid rollout mode %q for platform %q", mode, platform)
		}
	}
	for platform, groups := range c.Groups {
		if strings.TrimSpace(platform) == "" {
			return fmt.Errorf("rollout group platform must not be empty")
		}
		for groupID, mode := range groups {
			parsed, err := strconv.ParseInt(strings.TrimSpace(groupID), 10, 64)
			if err != nil || parsed <= 0 {
				return fmt.Errorf("invalid rollout group id %q for platform %q", groupID, platform)
			}
			if !validRolloutMode(mode) {
				return fmt.Errorf("invalid rollout mode %q for platform %q group %s", mode, platform, groupID)
			}
		}
	}
	return nil
}

func normalizeRolloutMode(mode RolloutMode) RolloutMode {
	switch RolloutMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case RolloutShadow:
		return RolloutShadow
	case RolloutEnforce:
		return RolloutEnforce
	default:
		return RolloutOff
	}
}

func validRolloutMode(mode RolloutMode) bool {
	switch mode {
	case RolloutOff, RolloutShadow, RolloutEnforce:
		return true
	default:
		return false
	}
}

func formatRolloutGroupID(groupID int64) string {
	return strconv.FormatInt(groupID, 10)
}
