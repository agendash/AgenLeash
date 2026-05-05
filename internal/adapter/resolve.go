package adapter

import (
	"fmt"

	"github.com/agendash/AgenLeash/internal/model"
)

func Resolve(spec AdapterSpec, version string) (EffectiveSpec, error) {
	if err := spec.Validate(); err != nil {
		return EffectiveSpec{}, err
	}

	effective := EffectiveSpec{
		AdapterName:  spec.Metadata.Name,
		AgentFamily:  spec.Spec.AgentFamily,
		Version:      version,
		Runtime:      cloneRuntime(spec.Spec.Runtime),
		CwdPolicy:    cloneCwdPolicy(spec.Spec.CwdPolicy),
		Capabilities: materializeCapabilities(spec.Spec.Capabilities),
		Features:     NormalizeFeatureSet(spec.Spec.Features),
		Conversation: cloneConversation(spec.Spec.Conversation),
		Workspace:    cloneWorkspace(spec.Spec.Workspace),
		EventParser:  cloneEventParser(spec.Spec.EventParser),
	}

	selectedProfile := ""
	if version == "" && spec.Spec.Detection.FallbackProfile != "" {
		if profile, ok := spec.profileByName(spec.Spec.Detection.FallbackProfile); ok {
			selectedProfile = profile.Name
			applyProfile(&effective, profile)
			effective.Profile = selectedProfile
			return effective, nil
		}
	}

	if version != "" {
		for _, profile := range spec.Spec.VersionProfiles {
			matched, err := ParseVersionRange(profile.Match)
			if err != nil {
				return EffectiveSpec{}, fmt.Errorf("profile %q: %w", profile.Name, err)
			}
			ok, err := matched.Matches(version)
			if err != nil {
				return EffectiveSpec{}, fmt.Errorf("profile %q: %w", profile.Name, err)
			}
			if ok {
				selectedProfile = profile.Name
				applyProfile(&effective, profile)
				break
			}
		}
	}

	if selectedProfile == "" && spec.Spec.Detection.FallbackProfile != "" {
		if profile, ok := spec.profileByName(spec.Spec.Detection.FallbackProfile); ok {
			selectedProfile = profile.Name
			applyProfile(&effective, profile)
		}
	}

	effective.Profile = selectedProfile
	return effective, nil
}

func (s AdapterSpec) profileByName(name string) (VersionProfile, bool) {
	for _, profile := range s.Spec.VersionProfiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return VersionProfile{}, false
}

func applyProfile(effective *EffectiveSpec, profile VersionProfile) {
	effective.Runtime = mergeRuntime(effective.Runtime, profile.Overrides.Runtime)
	effective.CwdPolicy = mergeCwdPolicy(effective.CwdPolicy, profile.Overrides.CwdPolicy)
	effective.Capabilities = mergeCapabilities(effective.Capabilities, profile.Overrides.Capabilities)
	effective.Features = mergeFeatures(effective.Features, profile.Overrides.Features)
	effective.Conversation = mergeConversation(effective.Conversation, profile.Overrides.Conversation)
	effective.Workspace = mergeWorkspace(effective.Workspace, profile.Overrides.Workspace)
	effective.EventParser = mergeEventParser(effective.EventParser, profile.Overrides.EventParser)
}

func materializeCapabilities(spec CapabilitySpec) model.Capabilities {
	return model.Capabilities{
		RequiresTTY:                boolValue(spec.RequiresTTY),
		RequiresRuntimeResize:      boolValue(spec.RequiresRuntimeResize),
		SupportsResume:             boolValue(spec.SupportsResume),
		SupportsInterrupt:          boolValue(spec.SupportsInterrupt),
		SupportsRawDebug:           boolValue(spec.SupportsRawDebug),
		SupportsWorkspaceSwitch:    boolValue(spec.SupportsWorkspaceSwitch),
		SupportsNativeConversation: boolValue(spec.SupportsNativeConversationID),
	}
}
