package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/session"
)

var ErrNoPendingQuestion = errors.New("no pending question")

type PendingQuestion struct {
	Question  string    `json:"question"`
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id"`
	Agent     string    `json:"agent"`
}

type questionAnswer struct {
	Answer    string    `json:"answer"`
	Timestamp time.Time `json:"timestamp"`
}

func pendingQuestionPath(metaDir string) string {
	return filepath.Join(metaDir, "question.json")
}

func pendingAnswerPath(metaDir string) string {
	return filepath.Join(metaDir, "answer.json")
}

func LoadPendingQuestion(sess *session.Session) (*PendingQuestion, error) {
	if sess == nil {
		return nil, nil
	}
	return loadPendingQuestionFromDir(sess.MetadataDirPath())
}

func LoadPendingQuestionInWorkspace(workspace, sessionID string) (*PendingQuestion, error) {
	return loadPendingQuestionFromDir(MetadataDir(workspace, sessionID))
}

func loadPendingQuestionFromDir(metaDir string) (*PendingQuestion, error) {
	if strings.TrimSpace(metaDir) == "" {
		return nil, nil
	}

	data, err := os.ReadFile(pendingQuestionPath(metaDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var question PendingQuestion
	if err := json.Unmarshal(data, &question); err != nil {
		return nil, err
	}
	return &question, nil
}

func SubmitPendingQuestionAnswer(sess *session.Session, answer string) error {
	if sess == nil {
		return ErrNoPendingQuestion
	}
	return submitPendingQuestionAnswerToDir(sess.MetadataDirPath(), answer)
}

func SubmitPendingQuestionAnswerInWorkspace(workspace, sessionID, answer string) error {
	return submitPendingQuestionAnswerToDir(MetadataDir(workspace, sessionID), answer)
}

func submitPendingQuestionAnswerToDir(metaDir, answer string) error {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return fmt.Errorf("answer is required")
	}
	if strings.TrimSpace(metaDir) == "" {
		return ErrNoPendingQuestion
	}

	question, err := loadPendingQuestionFromDir(metaDir)
	if err != nil {
		return err
	}
	if question == nil {
		return ErrNoPendingQuestion
	}

	payload, err := json.Marshal(questionAnswer{
		Answer:    answer,
		Timestamp: time.Now().UTC(),
	})
	if err != nil {
		return err
	}

	if err := os.MkdirAll(metaDir, 0700); err != nil {
		return err
	}

	answerPath := pendingAnswerPath(metaDir)
	tmpPath := answerPath + ".tmp"
	if err := os.WriteFile(tmpPath, append(payload, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, answerPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Remove(pendingQuestionPath(metaDir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
