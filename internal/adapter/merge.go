package adapter

import "github.com/agendash/AgenLeash/internal/model"

func boolValue(ptr *bool) bool {
	if ptr == nil {
		return false
	}
	return *ptr
}

func stringValue(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func cloneRuntime(in RuntimeSpec) RuntimeSpec {
	out := in
	if in.Args != nil {
		out.Args = append([]string(nil), in.Args...)
	}
	if in.Env != nil {
		out.Env = make(map[string]string, len(in.Env))
		for k, v := range in.Env {
			out.Env[k] = v
		}
	}
	return out
}

func mergeRuntime(base RuntimeSpec, override *RuntimeSpec) RuntimeSpec {
	if override == nil {
		return base
	}
	out := cloneRuntime(base)
	if override.Mode != nil {
		out.Mode = override.Mode
	}
	if override.Entrypoint != nil {
		out.Entrypoint = override.Entrypoint
	}
	if override.Args != nil {
		out.Args = append([]string(nil), override.Args...)
	}
	if override.Env != nil {
		out.Env = make(map[string]string, len(override.Env))
		for k, v := range override.Env {
			out.Env[k] = v
		}
	}
	if override.StartupTimeoutSec != nil {
		out.StartupTimeoutSec = override.StartupTimeoutSec
	}
	return out
}

func cloneCwdPolicy(in CwdPolicySpec) CwdPolicySpec {
	out := in
	if in.AllowedRoots != nil {
		out.AllowedRoots = append([]string(nil), in.AllowedRoots...)
	}
	return out
}

func mergeCwdPolicy(base CwdPolicySpec, override *CwdPolicySpec) CwdPolicySpec {
	if override == nil {
		return base
	}
	out := cloneCwdPolicy(base)
	if override.Mode != nil {
		out.Mode = override.Mode
	}
	if override.AllowedRoots != nil {
		out.AllowedRoots = append([]string(nil), override.AllowedRoots...)
	}
	if override.RequireGitRoot != nil {
		out.RequireGitRoot = override.RequireGitRoot
	}
	if override.ResolveSymlink != nil {
		out.ResolveSymlink = override.ResolveSymlink
	}
	return out
}

func mergeCapabilities(base model.Capabilities, override CapabilitySpec) model.Capabilities {
	if override.RequiresTTY != nil {
		base.RequiresTTY = *override.RequiresTTY
	}
	if override.RequiresRuntimeResize != nil {
		base.RequiresRuntimeResize = *override.RequiresRuntimeResize
	}
	if override.SupportsResume != nil {
		base.SupportsResume = *override.SupportsResume
	}
	if override.SupportsInterrupt != nil {
		base.SupportsInterrupt = *override.SupportsInterrupt
	}
	if override.SupportsRawDebug != nil {
		base.SupportsRawDebug = *override.SupportsRawDebug
	}
	if override.SupportsWorkspaceSwitch != nil {
		base.SupportsWorkspaceSwitch = *override.SupportsWorkspaceSwitch
	}
	if override.SupportsNativeConversationID != nil {
		base.SupportsNativeConversation = *override.SupportsNativeConversationID
	}
	return base
}

func mergeFeatures(base model.FeatureSet, override model.FeatureSet) model.FeatureSet {
	out := NormalizeFeatureSet(base)
	for key, value := range override {
		canonical := CanonicalFeatureKey(key)
		if canonical == "" {
			continue
		}
		out[canonical] = value
	}
	return out
}

func cloneConversation(in ConversationSpec) ConversationSpec {
	out := in
	if in.Extractors != nil {
		out.Extractors = append([]ConversationExtractor(nil), in.Extractors...)
	}
	return out
}

func mergeConversation(base ConversationSpec, override *ConversationSpec) ConversationSpec {
	if override == nil {
		return base
	}
	out := cloneConversation(base)
	if override.Mode != nil {
		out.Mode = override.Mode
	}
	if override.Extractors != nil {
		out.Extractors = append([]ConversationExtractor(nil), override.Extractors...)
	}
	return out
}

func cloneWorkspace(in WorkspaceSpec) WorkspaceSpec {
	return in
}

func mergeWorkspace(base WorkspaceSpec, override *WorkspaceSpec) WorkspaceSpec {
	if override == nil {
		return base
	}
	out := cloneWorkspace(base)
	if override.Mode != nil {
		out.Mode = override.Mode
	}
	return out
}

func cloneEventParser(in EventParserSpec) EventParserSpec {
	return in
}

func mergeEventParser(base EventParserSpec, override *EventParserSpec) EventParserSpec {
	if override == nil {
		return base
	}
	out := cloneEventParser(base)
	if override.Type != nil {
		out.Type = override.Type
	}
	if override.Profile != nil {
		out.Profile = override.Profile
	}
	return out
}
