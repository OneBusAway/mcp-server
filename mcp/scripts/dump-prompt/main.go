// Prints the transit_assistant system prompt to stdout.
// Used by scripts/eval.sh --full-prompt to produce evals/system_prompt.txt.
package main

import (
	"fmt"

	"oba-mcp/tools"
)

func main() {
	fmt.Print(tools.SystemPrompt())
}
