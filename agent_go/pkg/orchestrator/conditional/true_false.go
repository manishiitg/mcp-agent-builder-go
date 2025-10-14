package conditional

// GetPrompt returns a prompt for true/false decisions with reasoning
func GetPrompt(context, question string) string {
	return `You are a decision assistant. Analyze the context and return a true/false decision with reasoning.

Context: ` + context + `

Question: ` + question + `

Instructions:
1. You mainly need to determine answer to the question based on question.
2. Yes = true , No = false
3. Provide clear reasoning for your decision

Return ONLY valid JSON: {"result": true/false, "reason": "your reasoning here"}`
}

// GetSchema returns the JSON schema
func GetSchema() string {
	return `{
  "type": "object",
  "properties": {
    "result": {"type": "boolean"},
    "reason": {"type": "string"}
  },
  "required": ["result", "reason"],
  "additionalProperties": false
}`
}
