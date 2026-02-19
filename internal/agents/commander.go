package agents

import (
	"bore-tui/internal/db"
	"fmt"
	"sort"
	"strings"
)

// CommanderContext holds all the dynamic data injected into a Commander prompt.
type CommanderContext struct {
	Brain    []db.CommanderMemory
	Crews    []db.Crew
	Threads  []db.Thread
	PastRuns []db.AgentRun
	Lessons  []db.AgentLesson
}

// BuildCommanderSystemPrompt returns the Commander's system prompt with injected context.
func BuildCommanderSystemPrompt(ctx CommanderContext) string {
	var b strings.Builder

	b.WriteString(`You are **Commander**, the top-level orchestrator for a repo-centric local engineering system called **bore-tui**.

Your responsibilities:
1) Understand the user task
2) Look for relevant historical context from the SQLite DB summaries provided
3) Ask ONLY necessary clarifying questions
4) Propose 2-3 execution options with tradeoffs
5) After the user selects an option and a base branch, produce a final "Execution Brief"

Constraints:
- You do not edit code or run commands.
- You must keep the user safe: insist on a review period.
- You must be explicit about risks and scope.
- You must produce structured outputs exactly in the requested formats.
`)

	writeBrainSection(&b, ctx.Brain)
	writeCrewsSection(&b, ctx.Crews)
	writeThreadsSection(&b, ctx.Threads)
	writePastRunsSection(&b, ctx.PastRuns)
	writeLessonsSection(&b, ctx.Lessons)
	writeCommanderOutputFormats(&b)

	return b.String()
}

// BuildClarificationPrompt builds the user message asking Commander for clarifying questions.
func BuildClarificationPrompt(taskPrompt string) string {
	var b strings.Builder

	b.WriteString("## User Task\n\n")
	b.WriteString(taskPrompt)
	b.WriteString("\n\n")
	b.WriteString(`## Instructions

Review the task above along with the context provided in your system prompt.
If you need clarification before proposing execution options, respond with a JSON object containing your questions.
If the task is clear enough, respond with an empty questions array.

Respond with ONLY the following JSON (no markdown fences, no extra text):

{
  "type": "clarifications",
  "questions": [
    {
      "id": "q1",
      "question": "Your clarifying question here",
      "why": "Why this question matters for planning"
    }
  ]
}
`)

	return b.String()
}

// BuildOptionsPrompt builds the user message asking Commander for execution options.
// answers is the user's answers to clarifying questions (may be empty if no questions were asked).
func BuildOptionsPrompt(taskPrompt string, answers map[string]string) string {
	var b strings.Builder

	b.WriteString("## User Task\n\n")
	b.WriteString(taskPrompt)
	b.WriteString("\n\n")

	if len(answers) > 0 {
		b.WriteString("## Clarification Answers\n\n")
		keys := make([]string, 0, len(answers))
		for k := range answers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, id := range keys {
			fmt.Fprintf(&b, "- **%s**: %s\n", id, answers[id])
		}
		b.WriteString("\n")
	}

	b.WriteString(`## Instructions

Based on the task and any clarification answers above, propose 2-3 execution options with different tradeoffs.
For each option, include approach steps, a crew suggestion (or "none"), a worker budget suggestion, risks, and validation steps.

Respond with ONLY the following JSON (no markdown fences, no extra text):

{
  "type": "options",
  "options": [
    {
      "id": "opt1",
      "title": "Short title for the approach",
      "summary": "One-paragraph summary of the approach",
      "approach_steps": ["Step 1", "Step 2"],
      "crew_suggestion": "crew name or none",
      "worker_budget_suggestion": 2,
      "risks": ["Risk 1"],
      "validation": ["Validation step 1"]
    }
  ]
}
`)

	return b.String()
}

// BuildExecutionBriefPrompt builds the user message asking Commander for the final execution brief.
// selectedOptionID is the ID of the option the user selected.
// baseBranch is the branch the user chose.
func BuildExecutionBriefPrompt(taskPrompt string, selectedOptionID string, baseBranch string) string {
	var b strings.Builder

	b.WriteString("## User Task\n\n")
	b.WriteString(taskPrompt)
	b.WriteString("\n\n")

	fmt.Fprintf(&b, "## User Selection\n\n- **Selected option**: %s\n- **Base branch**: %s\n\n", selectedOptionID, baseBranch)

	b.WriteString(`## Instructions

The user has selected an option and a base branch. Produce the final execution brief that the Boss agent will use to carry out the work.
Include the scope, success criteria, crew assignment, worker budget, risks, and validation steps.

Respond with ONLY the following JSON (no markdown fences, no extra text):

{
  "type": "execution_brief",
  "selected_option_id": "opt1",
  "base_branch": "main",
  "thread": "thread name for grouping this task",
  "task_title": "Short descriptive title",
  "scope": ["What is in scope"],
  "not_in_scope": ["What is explicitly out of scope"],
  "success_criteria": ["Criterion 1"],
  "crew": "crew name or none",
  "worker_budget": 2,
  "key_risks": ["Risk 1"],
  "recommended_validation": ["Validation step 1"]
}
`)

	return b.String()
}

func writeBrainSection(b *strings.Builder, brain []db.CommanderMemory) {
	b.WriteString("\n## Commander Brain (Persistent Memory)\n\n")
	if len(brain) == 0 {
		b.WriteString("No persistent memory entries.\n")
		return
	}
	for _, m := range brain {
		fmt.Fprintf(b, "- **%s**: %s\n", m.Key, m.Value)
	}
}

func writeCrewsSection(b *strings.Builder, crews []db.Crew) {
	b.WriteString("\n## Available Crews\n\n")
	if len(crews) == 0 {
		b.WriteString("No crews defined. The task will run without crew constraints.\n")
		return
	}
	for _, c := range crews {
		fmt.Fprintf(b, "### %s\n", c.Name)
		fmt.Fprintf(b, "- **Objective**: %s\n", c.Objective)
		if c.Constraints != "" {
			fmt.Fprintf(b, "- **Constraints**: %s\n", c.Constraints)
		}
		if c.AllowedCommands != "" {
			fmt.Fprintf(b, "- **Allowed commands**: %s\n", c.AllowedCommands)
		}
		if c.OwnershipPaths != "" {
			fmt.Fprintf(b, "- **Ownership paths**: %s\n", c.OwnershipPaths)
		}
		b.WriteByte('\n')
	}
}

func writeThreadsSection(b *strings.Builder, threads []db.Thread) {
	b.WriteString("\n## Active Threads\n\n")
	if len(threads) == 0 {
		b.WriteString("No active threads.\n")
		return
	}
	for _, t := range threads {
		fmt.Fprintf(b, "- **%s**: %s\n", t.Name, t.Description)
	}
}

func writePastRunsSection(b *strings.Builder, runs []db.AgentRun) {
	b.WriteString("\n## Recent Past Runs\n\n")
	if len(runs) == 0 {
		b.WriteString("No recent past runs.\n")
		return
	}
	for _, r := range runs {
		fmt.Fprintf(b, "### %s (%s) - %s\n", r.Role, r.AgentType, r.Outcome)
		if r.Summary != "" {
			fmt.Fprintf(b, "%s\n", r.Summary)
		}
		if r.FilesChanged != "" {
			fmt.Fprintf(b, "- **Files changed**: %s\n", r.FilesChanged)
		}
		b.WriteByte('\n')
	}
}

func writeLessonsSection(b *strings.Builder, lessons []db.AgentLesson) {
	b.WriteString("\n## Lessons Learned\n\n")
	if len(lessons) == 0 {
		b.WriteString("No lessons recorded.\n")
		return
	}
	for _, l := range lessons {
		fmt.Fprintf(b, "- [%s] (%s): %s\n", l.LessonType, l.AgentType, l.Content)
	}
}

func writeCommanderOutputFormats(b *strings.Builder) {
	b.WriteString(`
## Output Format Rules

You will be asked to produce one of three output types per message. Each request will specify which format to use.
Always respond with ONLY the JSON object requested -- no markdown fences, no commentary, no extra text.

The three formats are:

1. **Clarifications** (type: "clarifications") -- asking the user clarifying questions
2. **Options** (type: "options") -- proposing 2-3 execution approaches
3. **Execution Brief** (type: "execution_brief") -- the final plan for the Boss agent
`)
}
