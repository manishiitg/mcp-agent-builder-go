package external

import (
	"fmt"
)

// DemoValidation shows examples of system prompt validation
func DemoValidation() {
	fmt.Println("=== System Prompt Validation Demo ===")

	// Test 1: Valid template
	fmt.Println("1. Valid Custom Template:")
	validTemplate := `You are a specialized assistant.

Available tools:
{{TOOLS}}

{{PROMPTS_SECTION}}

{{RESOURCES_SECTION}}

{{VIRTUAL_TOOLS_SECTION}}

Your role is to help users effectively.`

	if err := ValidateCustomTemplate(validTemplate); err != nil {
		fmt.Printf("❌ Validation failed: %v\n", err)
	} else {
		fmt.Println("✅ Validation passed")
	}

	// Test 2: Invalid template - missing all placeholders
	fmt.Println("\n2. Invalid Template (Missing All Placeholders):")
	invalidTemplate := `You are a custom assistant.
Use tools when needed.`

	if err := ValidateCustomTemplate(invalidTemplate); err != nil {
		fmt.Printf("❌ Validation failed: %v\n", err)
	} else {
		fmt.Println("✅ Validation passed")
	}

	// Test 3: Invalid template - missing some placeholders
	fmt.Println("\n3. Invalid Template (Missing Some Placeholders):")
	partialTemplate := `You are a custom assistant.

Available tools:
{{TOOLS}}

{{PROMPTS_SECTION}}

But missing resources and virtual tools.`

	if err := ValidateCustomTemplate(partialTemplate); err != nil {
		fmt.Printf("❌ Validation failed: %v\n", err)
	} else {
		fmt.Println("✅ Validation passed")
	}

	// Test 4: Show required placeholders
	fmt.Println("\n4. Required Placeholders:")
	required := []string{
		"{{TOOLS}}",
		"{{PROMPTS_SECTION}}",
		"{{RESOURCES_SECTION}}",
		"{{VIRTUAL_TOOLS_SECTION}}",
	}
	fmt.Println("All custom templates must include:")
	for i, placeholder := range required {
		fmt.Printf("  %d. %s\n", i+1, placeholder)
	}

	fmt.Println("\n=== Demo Complete ===")
}
