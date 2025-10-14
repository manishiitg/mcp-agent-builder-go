package llm

import "context"

// LLMCallFunc is a type-safe function signature for LLM calls.
type LLMCallFunc func(ctx context.Context, prompt string) (string, error)
