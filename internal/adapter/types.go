package adapter

import "github.com/agendash/AgenLeash/internal/model"

type AdapterSpec struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
	Spec       Spec     `json:"spec"`
}

type Metadata struct {
	Name            string `json:"name"`
	DisplayName     string `json:"displayName,omitempty"`
	SchemaVersion   int    `json:"schemaVersion"`
	AdapterRevision int    `json:"adapterRevision"`
}

type Spec struct {
	AgentFamily     string           `json:"agentFamily"`
	Detection       DetectionSpec    `json:"detection"`
	Runtime         RuntimeSpec      `json:"runtime"`
	CwdPolicy       CwdPolicySpec    `json:"cwdPolicy"`
	Capabilities    CapabilitySpec   `json:"capabilities"`
	Features        model.FeatureSet `json:"features,omitempty"`
	Conversation    ConversationSpec `json:"conversation"`
	Workspace       WorkspaceSpec    `json:"workspace"`
	EventParser     EventParserSpec  `json:"eventParser"`
	VersionProfiles []VersionProfile `json:"versionProfiles,omitempty"`
}

type DetectionSpec struct {
	BinaryNames     []string        `json:"binaryNames,omitempty"`
	VersionStrategy VersionStrategy `json:"versionStrategy"`
	FallbackProfile string          `json:"fallbackProfile,omitempty"`
}

type VersionStrategy struct {
	Type    string   `json:"type"`
	Command []string `json:"command,omitempty"`
	Regex   string   `json:"regex,omitempty"`
	Source  string   `json:"source,omitempty"`
	Path    string   `json:"path,omitempty"`
	Hook    string   `json:"hook,omitempty"`
}

type RuntimeSpec struct {
	Mode              *string           `json:"mode,omitempty"`
	Entrypoint        *string           `json:"entrypoint,omitempty"`
	Args              []string          `json:"args,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
	StartupTimeoutSec *int              `json:"startupTimeoutSec,omitempty"`
}

type CwdPolicySpec struct {
	Mode           *string  `json:"mode,omitempty"`
	AllowedRoots   []string `json:"allowedRoots,omitempty"`
	RequireGitRoot *bool    `json:"requireGitRoot,omitempty"`
	ResolveSymlink *bool    `json:"resolveSymlink,omitempty"`
}

type CapabilitySpec struct {
	RequiresTTY                  *bool `json:"requiresTTY,omitempty"`
	RequiresRuntimeResize        *bool `json:"requiresRuntimeResize,omitempty"`
	SupportsResume               *bool `json:"supportsResume,omitempty"`
	SupportsInterrupt            *bool `json:"supportsInterrupt,omitempty"`
	SupportsRawDebug             *bool `json:"supportsRawDebug,omitempty"`
	SupportsWorkspaceSwitch      *bool `json:"supportsWorkspaceSwitch,omitempty"`
	SupportsNativeConversationID *bool `json:"supportsNativeConversationId,omitempty"`
	SupportsStructuredOutput     *bool `json:"supportsStructuredOutput,omitempty"`
}

type ConversationSpec struct {
	Mode       *string                 `json:"mode,omitempty"`
	Extractors []ConversationExtractor `json:"extractors,omitempty"`
}

type ConversationExtractor struct {
	Type    string `json:"type"`
	Source  string `json:"source,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Path    string `json:"path,omitempty"`
}

type WorkspaceSpec struct {
	Mode *string `json:"mode,omitempty"`
}

type EventParserSpec struct {
	Type    *string `json:"type,omitempty"`
	Profile *string `json:"profile,omitempty"`
}

type VersionProfile struct {
	Name      string                  `json:"name"`
	Match     string                  `json:"match"`
	Overrides VersionProfileOverrides `json:"overrides"`
}

type VersionProfileOverrides struct {
	Runtime      *RuntimeSpec      `json:"runtime,omitempty"`
	CwdPolicy    *CwdPolicySpec    `json:"cwdPolicy,omitempty"`
	Capabilities CapabilitySpec    `json:"capabilities,omitempty"`
	Features     model.FeatureSet  `json:"features,omitempty"`
	Conversation *ConversationSpec `json:"conversation,omitempty"`
	Workspace    *WorkspaceSpec    `json:"workspace,omitempty"`
	EventParser  *EventParserSpec  `json:"eventParser,omitempty"`
}

type EffectiveSpec struct {
	AdapterName  string
	AgentFamily  string
	Version      string
	Profile      string
	Runtime      RuntimeSpec
	CwdPolicy    CwdPolicySpec
	Capabilities model.Capabilities
	Features     model.FeatureSet
	Conversation ConversationSpec
	Workspace    WorkspaceSpec
	EventParser  EventParserSpec
}

func (s AdapterSpec) Validate() error {
	switch {
	case s.APIVersion == "":
		return ErrInvalidSpec("apiVersion is required")
	case s.Kind != "AdapterSpec":
		return ErrInvalidSpec("kind must be AdapterSpec")
	case s.Metadata.Name == "":
		return ErrInvalidSpec("metadata.name is required")
	case s.Metadata.SchemaVersion <= 0:
		return ErrInvalidSpec("metadata.schemaVersion must be positive")
	case s.Metadata.AdapterRevision <= 0:
		return ErrInvalidSpec("metadata.adapterRevision must be positive")
	case s.Spec.AgentFamily == "":
		return ErrInvalidSpec("spec.agentFamily is required")
	case s.Spec.Detection.VersionStrategy.Type == "":
		return ErrInvalidSpec("spec.detection.versionStrategy.type is required")
	case s.Spec.Runtime.Mode == nil || *s.Spec.Runtime.Mode == "":
		return ErrInvalidSpec("spec.runtime.mode is required")
	case s.Spec.CwdPolicy.Mode == nil || *s.Spec.CwdPolicy.Mode == "":
		return ErrInvalidSpec("spec.cwdPolicy.mode is required")
	case s.Spec.Conversation.Mode == nil || *s.Spec.Conversation.Mode == "":
		return ErrInvalidSpec("spec.conversation.mode is required")
	case s.Spec.Workspace.Mode == nil || *s.Spec.Workspace.Mode == "":
		return ErrInvalidSpec("spec.workspace.mode is required")
	case s.Spec.EventParser.Type == nil || *s.Spec.EventParser.Type == "":
		return ErrInvalidSpec("spec.eventParser.type is required")
	}

	for _, profile := range s.Spec.VersionProfiles {
		if profile.Name == "" {
			return ErrInvalidSpec("spec.versionProfiles[].name is required")
		}
		if profile.Match == "" {
			return ErrInvalidSpec("spec.versionProfiles[].match is required")
		}
		if _, err := ParseVersionRange(profile.Match); err != nil {
			return err
		}
	}

	return nil
}
