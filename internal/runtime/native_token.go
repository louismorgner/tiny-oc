package runtime

import (
	"fmt"
	"strings"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

// tokenCodec is a lazily-initialized tiktoken encoder (cl100k_base).
// cl100k_base is the closest publicly available encoding to what Claude
// models use and gives much better estimates than the 4-byte heuristic.
var (
	tokenCodec     tokenizer.Codec
	tokenCodecOnce sync.Once
)

// getTokenCodec returns the shared tokenizer, initializing it on first call.
// If initialization fails, the codec stays nil permanently (sync.Once won't
// retry) and callers fall back to the len/4 heuristic.
func getTokenCodec() tokenizer.Codec {
	tokenCodecOnce.Do(func() {
		enc, err := tokenizer.Get(tokenizer.Cl100kBase)
		if err == nil {
			tokenCodec = enc
		}
	})
	return tokenCodec
}

// approxBytesPerToken is the fallback ratio when the tokenizer is unavailable.
const approxBytesPerToken = 4

// estimateTokens returns a token count for the given string using the
// cl100k_base tokenizer. Falls back to len/4 if the tokenizer is unavailable.
func estimateTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	if codec := getTokenCodec(); codec != nil {
		n, err := codec.Count(s)
		if err == nil {
			return n
		}
	}
	return (len(s) + approxBytesPerToken - 1) / approxBytesPerToken
}

// estimateMessagesTokens returns a rough total token count for a slice of
// messages, including per-message overhead (role tags, separators, etc.).
// The overhead of ~4 tokens per message accounts for role/name/separator
// tokens that the tokenizer inserts around each message.
const perMessageOverhead = 4

func estimateMessagesTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateTokens(msg.Content) + perMessageOverhead
		total += estimateTokens(msg.Name)
		for _, call := range msg.ToolCalls {
			total += estimateTokens(call.Function.Name)
			total += estimateTokens(call.Function.Arguments)
			total += perMessageOverhead
		}
	}
	return total
}

// truncateMiddle keeps the first `prefixBytes` and last `suffixBytes` of s,
// replacing the middle with a marker showing how much was dropped.
// This preserves context boundaries — the beginning (usually headers,
// function signatures, path info) and the end (usually the most recent
// or relevant output) are both kept.
//
// Claude Code uses this same strategy: their truncate_middle_with_token_budget
// keeps prefix+suffix and drops the middle, since LLMs attend to beginnings
// and endings more strongly than middles (the "lost in the middle" effect).
func truncateMiddle(s string, maxBytes int) string {
	if len(s) <= maxBytes || maxBytes <= 0 {
		return s
	}

	// Reserve space for the marker line itself (~80 chars max).
	markerReserve := 80
	if maxBytes <= markerReserve*2 {
		// Too small for middle truncation — fall back to simple tail cut.
		return s[:maxBytes] + "\n...[truncated]"
	}

	usable := maxBytes - markerReserve
	prefixLen := usable * 2 / 3 // 2/3 prefix, 1/3 suffix — beginning is usually more informative
	suffixLen := usable - prefixLen

	// Snap to newline boundaries to avoid cutting mid-line.
	prefix := s[:prefixLen]
	if idx := strings.LastIndex(prefix, "\n"); idx > prefixLen/2 {
		prefix = s[:idx+1]
	}

	suffix := s[len(s)-suffixLen:]
	if idx := strings.Index(suffix, "\n"); idx >= 0 && idx < suffixLen/2 {
		suffix = s[len(s)-suffixLen+idx+1:]
	}

	droppedBytes := len(s) - len(prefix) - len(suffix)
	droppedTokens := estimateTokens(s[len(prefix) : len(s)-len(suffix)])
	marker := fmt.Sprintf("\n...[%d bytes / ~%d tokens truncated]...\n", droppedBytes, droppedTokens)

	return prefix + marker + suffix
}

// Per-tool token budget defaults. These control how much of each tool's
// output we keep in conversation history. The budgets are set based on
// the typical signal-to-noise ratio of each tool:
//
//   - Read: Files being read are usually important context. Keep more.
//   - Bash: Output can be huge (build logs, test output). Often only
//     the last portion (errors, summary) matters.
//   - Grep: Many matching lines, but usually only a few are relevant.
//     The model can re-grep if it needs more.
//   - Glob: File listings. Rarely need more than a screen's worth.
//   - Skill: Skill instructions are important — keep most of them.
//   - SubAgent: Agent output varies — keep a moderate amount.
//
// These are byte limits, not token limits. At ~4 bytes/token:
//   - 32KB ≈ 8K tokens
//   - 16KB ≈ 4K tokens
//   - 8KB  ≈ 2K tokens
var toolOutputBudgets = map[string]int{
	"Read":     48 * 1024, // 48KB — files are usually important context
	"Write":    1024,      // 1KB — confirmations are short
	"Edit":     1024,      // 1KB — confirmations are short
	"Bash":     32 * 1024, // 32KB — build/test output, keep more of the end
	"Grep":     16 * 1024, // 16KB — matching lines, model can re-grep
	"Glob":     8 * 1024,  // 8KB — file listings
	"Skill":    48 * 1024, // 48KB — instructions are important
	"SubAgent": 32 * 1024, // 32KB — agent deliverables
}

const defaultToolOutputBudget = 32 * 1024

// toolOutputBudget returns the max output bytes for the given tool name.
func toolOutputBudget(toolName string) int {
	if budget, ok := toolOutputBudgets[toolName]; ok {
		return budget
	}
	return defaultToolOutputBudget
}

// truncateToolOutput applies tool-specific truncation using middle-truncation.
// For Bash, it biases toward keeping the suffix (errors/results appear at the end).
func truncateToolOutput(toolName, output string) string {
	budget := toolOutputBudget(toolName)
	if len(output) <= budget {
		return output
	}
	return truncateMiddle(output, budget)
}
