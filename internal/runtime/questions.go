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

type PendingQuestionInfo struct {
	Question      *PendingQuestion `json:"question,omitempty"`
	AnswerPending bool             `json:"answer_pending,omitempty"`
	Error         string           `json:"error,omitempty"`
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
	info, err := InspectPendingQuestion(sess)
	if err != nil || info == nil {
		return nil, err
	}
	if info.Error != "" {
		return nil, fmt.Errorf("%s", info.Error)
	}
	return info.Question, nil
}

func LoadPendingQuestionInWorkspace(workspace, sessionID string) (*PendingQuestion, error) {
	info, err := InspectPendingQuestionInWorkspace(workspace, sessionID)
	if err != nil || info == nil {
		return nil, err
	}
	if info.Error != "" {
		return nil, fmt.Errorf("%s", info.Error)
	}
	return info.Question, nil
}

func InspectPendingQuestion(sess *session.Session) (*PendingQuestionInfo, error) {
	if sess == nil {
		return nil, nil
	}
	return inspectPendingQuestionFromDir(sess.MetadataDirPath())
}

func InspectPendingQuestionInWorkspace(workspace, sessionID string) (*PendingQuestionInfo, error) {
	return inspectPendingQuestionFromDir(MetadataDir(workspace, sessionID))
}

func inspectPendingQuestionFromDir(metaDir string) (*PendingQuestionInfo, error) {
	if strings.TrimSpace(metaDir) == "" {
		return nil, nil
	}

	info := &PendingQuestionInfo{}
	if _, err := os.Stat(pendingAnswerPath(metaDir)); err == nil {
		info.AnswerPending = true
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
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
		info.Error = fmt.Sprintf("failed to parse question.json: %v", err)
		return info, nil
	}
	info.Question = &question
	return info, nil
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

	info, err := inspectPendingQuestionFromDir(metaDir)
	if err != nil {
		return err
	}
	if info == nil {
		return ErrNoPendingQuestion
	}
	if info.Error != "" {
		return fmt.Errorf("%s", info.Error)
	}
	if info.Question == nil {
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
	return nil
}
