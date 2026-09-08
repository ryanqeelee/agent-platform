package agent

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/searchutil"
	"github.com/Tencent/WeKnora/internal/types"
)

const agentRetrievedImageRequirementMarker = "## Retrieved Image Output Requirement"

const agentRetrievedImageSystemRequirement = `

## Retrieved Image Output Requirement
Retrieved tool results for this turn contain Markdown images.
- Include a retrieved Markdown image only when it directly supports the user's requested answer, unless the user requests text-only output. Omit unrelated images, including sample forms used merely to illustrate a generic concept. Text-only answers are valid even when retrieval contains images.
- If an image is relevant and useful, copy its Markdown syntax verbatim. Preserve its complete URL exactly; never invent, shorten, normalize, or replace it.
- Use ASCII half-width parentheses exactly as ![alt](url); never use full-width （ or ）.
- Place each image immediately after the paragraph it supports.
- Before finishing, silently verify that each included image supports a specific claim or requested illustration; remove any that do not.`

func stepContainsMarkdownImage(step types.AgentStep) bool {
	for _, toolCall := range step.ToolCalls {
		if toolCall.Result != nil &&
			toolCall.Result.Success &&
			searchutil.MarkdownImageRegex.MatchString(toolCall.Result.Output) {
			return true
		}
	}
	return false
}

func appendAgentRetrievedImageRequirement(messages []chat.Message) []chat.Message {
	for _, message := range messages {
		if strings.Contains(message.Content, agentRetrievedImageRequirementMarker) {
			return messages
		}
	}
	// Append after the current prefix instead of editing the system prompt.
	// Provider prompt caches are prefix matches: mutating the system block
	// invalidates the tools and the whole transcript for every later round.
	return append(messages, chat.Message{
		Role:    "user",
		Content: strings.TrimSpace(agentRetrievedImageSystemRequirement),
	})
}
