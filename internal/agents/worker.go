package agents

import (
	"fmt"
	"strings"
)

// WorkerContext holds dynamic data for a Worker prompt.
type WorkerContext struct {
	Role            string
	Goal            string
	FilesOrPaths    []string
	AllowedCommands []string
	SuccessCriteria []string
	CrewObjective   string
	CrewConstraints string
}

// BuildWorkerSystemPrompt returns the Worker's system prompt with injected context.
func BuildWorkerSystemPrompt(ctx WorkerContext) string {
	var b strings.Builder

	b.WriteString(`You are a **Worker** agent for bore-tui. You operate inside a Git worktree directory for a single execution.

You are given:
- A narrow role and goal
- Target files/paths
- Allowed commands to run
- Success criteria

Your responsibilities:
1) Make the required code changes in the repository
2) Run the allowed validation commands
3) Report results
4) Keep changes minimal and aligned to the crew objective

Constraints:
- Work only in the current directory (the worktree). Do not reference outside paths.
- Do not modify unrelated files.
- If you need additional info, state it in the output under "blockers".
- Output must be structured JSON only.
`)

	writeWorkerContextSection(&b, ctx)
	writeWorkerOutputFormat(&b)

	return b.String()
}

func writeWorkerContextSection(b *strings.Builder, ctx WorkerContext) {
	b.WriteString("\n## Worker Assignment\n\n")
	fmt.Fprintf(b, "- **Role**: %s\n", ctx.Role)
	fmt.Fprintf(b, "- **Goal**: %s\n", ctx.Goal)

	if len(ctx.FilesOrPaths) > 0 {
		b.WriteString("- **Target files/paths**:\n")
		for _, f := range ctx.FilesOrPaths {
			fmt.Fprintf(b, "  - %s\n", f)
		}
	}

	if len(ctx.AllowedCommands) > 0 {
		b.WriteString("- **Allowed commands**:\n")
		for _, c := range ctx.AllowedCommands {
			fmt.Fprintf(b, "  - `%s`\n", c)
		}
	}

	if len(ctx.SuccessCriteria) > 0 {
		b.WriteString("- **Success criteria**:\n")
		for _, s := range ctx.SuccessCriteria {
			fmt.Fprintf(b, "  - %s\n", s)
		}
	}

	if ctx.CrewObjective != "" {
		fmt.Fprintf(b, "- **Crew objective**: %s\n", ctx.CrewObjective)
	}

	if ctx.CrewConstraints != "" {
		fmt.Fprintf(b, "- **Crew constraints**: %s\n", ctx.CrewConstraints)
	}
}

func writeWorkerOutputFormat(b *strings.Builder) {
	b.WriteString(`
## Output Format

When you have completed your work, respond with ONLY the following JSON (no markdown fences, no extra text):

{
  "type": "worker_result",
  "outcome": "success | partial | failed",
  "summary": "Brief summary of what you did",
  "files_changed": ["list/of/files/changed.go"],
  "commands_run": ["commands you executed"],
  "validation_results": ["result of each validation command"],
  "notes": ["any relevant observations"],
  "blockers": ["anything that prevented full completion"]
}
`)
}
