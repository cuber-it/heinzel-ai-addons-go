// Central registry of all context bag keys.
// Every addon MUST use these constants instead of string literals.
package addons

const (
	// Strategy and flow control
	KeyStrategy        = "strategy"
	KeyStrategyOverride = "strategy_override"
	KeyNeedsRerun      = "needs_rerun"
	KeyExecutePlanStep = "execute_plan_step"
	KeyPlanMode        = "plan_mode"
	KeyPlanCreating    = "plan_creating"

	// Output handling
	KeyOutputStreamed      = "output_streamed"
	KeyLastOutputForReview = "last_output_for_review"

	// Web search
	KeyNeedsWebSearch   = "needs_web_search"
	KeyWebFallbackTried = "web_fallback_tried"
	KeyWebFallbackDone  = "web_fallback_done"

	// User settings
	KeySessionName    = "session_name"
	KeyThinkingVisible = "thinking_visible"
	KeyActiveSkill    = "active_skill"
	KeyBtwQuestion    = "btw_question"

	// Resources
	KeyToolRegistry   = "tool_registry"
	KeyAttachedImages = "attached_images"

	// Token tracking
	KeyTurnTokensIn   = "turn_tokens_in"
	KeyTurnTokensOut  = "turn_tokens_out"
	KeyTurnCost       = "turn_cost"
	KeyLLMTokensIn    = "llm_tokens_in"
	KeyLLMTokensOut   = "llm_tokens_out"
	KeyMaxOutputTokens = "max_output_tokens"

	// Reasoning
	KeyThinkingDepth   = "thinking_depth"    // int: 0-5, controls reasoning granularity
	KeyNativeReasoning = "native_reasoning"  // bool: tell provider to use model's native reasoning

	// Compaction
	KeyCompactionLastMsgCount  = "compaction_last_msg_count"
	KeyCompactionLastExtracted = "compaction_last_extracted"

	// Internals
	KeyInternalQuery  = "internal_query"
	KeyThinking       = "thinking"
	KeyLastTurnNumber = "last_turn_number"
)
