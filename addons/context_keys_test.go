package addons

import (
	"reflect"
	"testing"
)

func TestAllExportedKeyConstantsAreNonEmpty(test *testing.T) {
	keys := map[string]string{
		"KeyStrategy":                KeyStrategy,
		"KeyStrategyOverride":        KeyStrategyOverride,
		"KeyNeedsRerun":              KeyNeedsRerun,
		"KeyExecutePlanStep":         KeyExecutePlanStep,
		"KeyPlanMode":                KeyPlanMode,
		"KeyPlanCreating":            KeyPlanCreating,
		"KeyOutputStreamed":           KeyOutputStreamed,
		"KeyLastOutputForReview":     KeyLastOutputForReview,
		"KeyNeedsWebSearch":          KeyNeedsWebSearch,
		"KeyWebFallbackTried":        KeyWebFallbackTried,
		"KeyWebFallbackDone":         KeyWebFallbackDone,
		"KeySessionName":             KeySessionName,
		"KeyThinkingVisible":         KeyThinkingVisible,
		"KeyActiveSkill":             KeyActiveSkill,
		"KeyBtwQuestion":             KeyBtwQuestion,
		"KeyToolRegistry":            KeyToolRegistry,
		"KeyAttachedImages":          KeyAttachedImages,
		"KeyTurnTokensIn":            KeyTurnTokensIn,
		"KeyTurnTokensOut":           KeyTurnTokensOut,
		"KeyTurnCost":                KeyTurnCost,
		"KeyLLMTokensIn":             KeyLLMTokensIn,
		"KeyLLMTokensOut":            KeyLLMTokensOut,
		"KeyMaxOutputTokens":         KeyMaxOutputTokens,
		"KeyThinkingDepth":           KeyThinkingDepth,
		"KeyNativeReasoning":         KeyNativeReasoning,
		"KeyCompactionLastMsgCount":  KeyCompactionLastMsgCount,
		"KeyCompactionLastExtracted": KeyCompactionLastExtracted,
		"KeyInternalQuery":           KeyInternalQuery,
		"KeyThinking":                KeyThinking,
		"KeyLastTurnNumber":          KeyLastTurnNumber,
	}

	for name, value := range keys {
		if value == "" {
			test.Errorf("key constant %s is empty", name)
		}
	}
}

func TestNoDuplicateKeyValues(test *testing.T) {
	keys := map[string]string{
		"KeyStrategy":                KeyStrategy,
		"KeyStrategyOverride":        KeyStrategyOverride,
		"KeyNeedsRerun":              KeyNeedsRerun,
		"KeyExecutePlanStep":         KeyExecutePlanStep,
		"KeyPlanMode":                KeyPlanMode,
		"KeyPlanCreating":            KeyPlanCreating,
		"KeyOutputStreamed":           KeyOutputStreamed,
		"KeyLastOutputForReview":     KeyLastOutputForReview,
		"KeyNeedsWebSearch":          KeyNeedsWebSearch,
		"KeyWebFallbackTried":        KeyWebFallbackTried,
		"KeyWebFallbackDone":         KeyWebFallbackDone,
		"KeySessionName":             KeySessionName,
		"KeyThinkingVisible":         KeyThinkingVisible,
		"KeyActiveSkill":             KeyActiveSkill,
		"KeyBtwQuestion":             KeyBtwQuestion,
		"KeyToolRegistry":            KeyToolRegistry,
		"KeyAttachedImages":          KeyAttachedImages,
		"KeyTurnTokensIn":            KeyTurnTokensIn,
		"KeyTurnTokensOut":           KeyTurnTokensOut,
		"KeyTurnCost":                KeyTurnCost,
		"KeyLLMTokensIn":             KeyLLMTokensIn,
		"KeyLLMTokensOut":            KeyLLMTokensOut,
		"KeyMaxOutputTokens":         KeyMaxOutputTokens,
		"KeyThinkingDepth":           KeyThinkingDepth,
		"KeyNativeReasoning":         KeyNativeReasoning,
		"KeyCompactionLastMsgCount":  KeyCompactionLastMsgCount,
		"KeyCompactionLastExtracted": KeyCompactionLastExtracted,
		"KeyInternalQuery":           KeyInternalQuery,
		"KeyThinking":                KeyThinking,
		"KeyLastTurnNumber":          KeyLastTurnNumber,
	}

	seen := map[string]string{} // value → first constant name
	for name, value := range keys {
		if first, duplicate := seen[value]; duplicate {
			test.Errorf("duplicate key value %q: %s and %s", value, first, name)
		}
		seen[value] = name
	}
}

func TestKeyConstantsAreStrings(test *testing.T) {
	// Verify the type is string via reflection on a representative constant
	keyType := reflect.TypeOf(KeyStrategy)
	if keyType.Kind() != reflect.String {
		test.Errorf("expected string kind, got %v", keyType.Kind())
	}
}
