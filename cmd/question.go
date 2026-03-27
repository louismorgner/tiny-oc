package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

var answerText string

func init() {
	questionCmd.Flags().Bool("json", false, "Output structured JSON")
	questionCmd.ValidArgsFunction = completeSessionIDs
	rootCmd.AddCommand(questionCmd)

	answerCmd.Flags().StringVar(&answerText, "text", "", "Answer text to submit")
	answerCmd.Flags().Bool("json", false, "Output structured JSON")
	answerCmd.ValidArgsFunction = completeSessionIDs
	rootCmd.AddCommand(answerCmd)
}

type pendingQuestionRow struct {
	ID                   string                   `json:"id"`
	Name                 string                   `json:"name,omitempty"`
	Agent                string                   `json:"agent"`
	Status               string                   `json:"status"`
	PendingQuestion      *runtime.PendingQuestion `json:"pending_question,omitempty"`
	AnswerPending        bool                     `json:"answer_pending,omitempty"`
	PendingQuestionError string                   `json:"pending_question_error,omitempty"`
}

var questionCmd = &cobra.Command{
	Use:   "question [session-id]",
	Short: "Inspect pending session questions",
	Long:  "Without arguments, list all pending operator questions in the current workspace. With a session ID, show the pending question for that session.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if len(args) == 0 {
			return listPendingQuestions(jsonFlag)
		}
		return showPendingQuestion(args[0], jsonFlag)
	},
}

var answerCmd = &cobra.Command{
	Use:   "answer <session-id>",
	Short: "Submit an answer for a pending session question",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}
		if strings.TrimSpace(answerText) == "" {
			return fmt.Errorf("required flag \"text\" not set")
		}

		sess, err := session.FindByIDOrPrefix(args[0])
		if err != nil {
			return err
		}
		if err := runtime.SubmitPendingQuestionAnswer(sess, answerText); err != nil {
			if errors.Is(err, runtime.ErrNoPendingQuestion) {
				return fmt.Errorf("session '%s' has no pending question", sess.ID)
			}
			return err
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			data, err := json.Marshal(pendingQuestionRow{
				ID:     sess.ID,
				Name:   sess.Name,
				Agent:  sess.Agent,
				Status: sess.ResolvedStatus(),
			})
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Println()
		ui.Success("Submitted answer for %s", ui.Cyan(sess.ID))
		ui.Info("Agent: %s", ui.Cyan(sess.Agent))
		ui.Info("Resume check: %s", ui.Bold(fmt.Sprintf("toc debug %s", sess.ID)))
		fmt.Println()
		return nil
	},
}

func listPendingQuestions(jsonFlag bool) error {
	sf, err := session.Load()
	if err != nil {
		return err
	}

	var rows []pendingQuestionRow
	for _, sess := range sf.Sessions {
		info, err := runtime.InspectPendingQuestion(&sess)
		if err != nil {
			return err
		}
		if info == nil {
			continue
		}
		rows = append(rows, pendingQuestionRow{
			ID:                   sess.ID,
			Name:                 sess.Name,
			Agent:                sess.Agent,
			Status:               sess.ResolvedStatus(),
			PendingQuestion:      info.Question,
			AnswerPending:        info.AnswerPending,
			PendingQuestionError: info.Error,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		left := time.Time{}
		if rows[i].PendingQuestion != nil {
			left = rows[i].PendingQuestion.Timestamp
		}
		right := time.Time{}
		if rows[j].PendingQuestion != nil {
			right = rows[j].PendingQuestion.Timestamp
		}
		return left.After(right)
	})

	if jsonFlag {
		data, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(rows) == 0 {
		ui.Info("No pending questions.")
		return nil
	}

	fmt.Println()
	fmt.Printf("  %-10s %-16s %-8s %-8s %s\n", ui.Bold("SESSION"), ui.Bold("AGENT"), ui.Bold("STATUS"), ui.Bold("ASKED"), ui.Bold("QUESTION"))
	fmt.Printf("  %-10s %-16s %-8s %-8s %s\n", ui.Dim("──────────"), ui.Dim("────────────────"), ui.Dim("────────"), ui.Dim("────────"), ui.Dim("────────────────────────────────────────"))
	for _, row := range rows {
		question := row.PendingQuestionError
		if row.PendingQuestion != nil {
			question = row.PendingQuestion.Question
		}
		if row.AnswerPending {
			question = "[answered] " + question
		}
		if len(question) > 52 {
			question = question[:49] + "..."
		}
		asked := "unknown"
		if row.PendingQuestion != nil {
			asked = formatQuestionAge(row.PendingQuestion.Timestamp)
		}
		fmt.Printf("  %-10s %-16s %-8s %-8s %s\n", ui.Dim(row.ID[:8]), ui.Cyan(row.Agent), ui.Dim(row.Status), ui.Dim(asked), ui.Dim(question))
	}
	fmt.Println()
	return nil
}

func showPendingQuestion(sessionID string, jsonFlag bool) error {
	sess, err := session.FindByIDOrPrefix(sessionID)
	if err != nil {
		return err
	}

	info, err := runtime.InspectPendingQuestion(sess)
	if err != nil {
		return err
	}
	if info == nil {
		return fmt.Errorf("session '%s' has no pending question", sess.ID)
	}

	row := pendingQuestionRow{
		ID:                   sess.ID,
		Name:                 sess.Name,
		Agent:                sess.Agent,
		Status:               sess.ResolvedStatus(),
		PendingQuestion:      info.Question,
		AnswerPending:        info.AnswerPending,
		PendingQuestionError: info.Error,
	}

	if jsonFlag {
		data, err := json.Marshal(row)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Println()
	fmt.Printf("  %s %s\n", ui.Bold("Session:"), ui.Cyan(sess.ID))
	fmt.Printf("  %s %s\n", ui.Bold("Agent:"), ui.Cyan(sess.Agent))
	fmt.Printf("  %s %s\n", ui.Bold("Status:"), ui.Dim(sess.ResolvedStatus()))
	if row.PendingQuestionError != "" {
		fmt.Printf("  %s %s\n", ui.Bold("Question error:"), ui.Dim(row.PendingQuestionError))
	} else if row.PendingQuestion != nil && !row.PendingQuestion.Timestamp.IsZero() {
		fmt.Printf("  %s %s\n", ui.Bold("Asked:"), ui.Dim(row.PendingQuestion.Timestamp.Format(time.RFC3339)))
	}
	if row.PendingQuestion != nil {
		fmt.Printf("  %s %s\n", ui.Bold("Question:"), ui.Dim(row.PendingQuestion.Question))
	}
	if row.AnswerPending {
		fmt.Printf("  %s %s\n", ui.Bold("Answer status:"), ui.Dim("answer submitted, waiting for session to consume it"))
	}
	fmt.Println()
	ui.Info("Answer with: %s", ui.Bold(fmt.Sprintf("toc answer %s --text \"...\"", sess.ID)))
	fmt.Println()
	return nil
}

func formatQuestionAge(ts time.Time) string {
	if ts.IsZero() {
		return "unknown"
	}
	d := time.Since(ts)
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
