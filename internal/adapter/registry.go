package adapter

import (
	"sort"
	"strings"

	"github.com/agendash/AgenLeash/internal/model"
)

var standardFeatureKeys = []string{
	"streamingText",
	"jsonEventStream",
	"fileReferences",
	"artifactOutput",
	"toolCallEvents",
	"commandExecutionEvents",
	"structuredPatch",
	"planMode",
	"approvalRequests",
	"imageInput",
	"multiWorkspace",
}

var featureAliases = map[string]string{
	"streaming_text":           "streamingText",
	"json_event_stream":        "jsonEventStream",
	"file_references":          "fileReferences",
	"artifact_output":          "artifactOutput",
	"tool_call_events":         "toolCallEvents",
	"command_execution_events": "commandExecutionEvents",
	"structured_patch":         "structuredPatch",
	"plan_mode":                "planMode",
	"approval_requests":        "approvalRequests",
	"image_input":              "imageInput",
	"multi_workspace":          "multiWorkspace",
}

func CanonicalFeatureKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if canonical, ok := featureAliases[key]; ok {
		return canonical
	}
	return key
}

func StandardFeatureRegistry() []string {
	out := make([]string, len(standardFeatureKeys))
	copy(out, standardFeatureKeys)
	sort.Strings(out)
	return out
}

func NormalizeFeatureSet(features model.FeatureSet) model.FeatureSet {
	if features == nil {
		features = model.FeatureSet{}
	}

	out := make(model.FeatureSet, len(features)+len(standardFeatureKeys))
	for _, key := range standardFeatureKeys {
		out[key] = false
	}
	for key, value := range features {
		canonical := CanonicalFeatureKey(key)
		if canonical == "" {
			continue
		}
		out[canonical] = value
	}
	return out
}
