package runtime

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tiny-oc/toc/internal/session"
)

func EventLogPath(sess *session.Session) string {
	if dir := sess.MetadataDirPath(); dir != "" {
		return filepath.Join(dir, "events.jsonl")
	}
	return ""
}

func EventLogPathInWorkspace(workspace, sessionID string) string {
	return filepath.Join(MetadataDir(workspace, sessionID), "events.jsonl")
}

func LoadEventLog(sess *session.Session) (*ParsedLog, error) {
	path := EventLogPath(sess)
	if path == "" {
		return nil, fmt.Errorf("session '%s' has no metadata directory for event storage", sess.ID)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := &ParsedLog{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		result.Events = append(result.Events, event)
		result.Steps = append(result.Steps, event.Step)
		if !event.Timestamp.IsZero() {
			if result.FirstTS.IsZero() {
				result.FirstTS = event.Timestamp
			}
			result.LastTS = event.Timestamp
		}
	}
	return result, nil
}

func SaveEventLog(sess *session.Session, parsed *ParsedLog) error {
	path := EventLogPath(sess)
	if path == "" {
		return fmt.Errorf("session '%s' has no metadata directory for event storage", sess.ID)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	events := parsed.Events
	if len(events) == 0 && len(parsed.Steps) > 0 {
		for _, step := range parsed.Steps {
			events = append(events, Event{Step: step})
		}
	}

	w := bufio.NewWriter(f)
	for _, event := range events {
		line, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return w.Flush()
}

func AppendEvent(sess *session.Session, event Event) error {
	path := EventLogPath(sess)
	if path == "" {
		return fmt.Errorf("session '%s' has no metadata directory for event storage", sess.ID)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	line, err := json.Marshal(event)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(append(line, '\n'))
	return err
}

func EventCount(sess *session.Session) int {
	parsed, err := LoadEventLog(sess)
	if err != nil {
		return 0
	}
	return len(parsed.Events)
}

func EnsureEventLog(sess *session.Session) (*ParsedLog, error) {
	status := sess.ResolvedStatus()

	if status == session.StatusActive {
		if path := EventLogPath(sess); path != "" {
			if _, err := os.Stat(path); err == nil {
				return LoadEventLog(sess)
			}
		}
	}

	provider, err := Get(sess.RuntimeName())
	if err != nil {
		return nil, err
	}
	logPath := provider.SessionLogPath(sess)
	if logPath == "" {
		return nil, fmt.Errorf("could not resolve session log for session '%s'", sess.ID)
	}

	parsed, err := provider.ParseSessionLog(logPath)
	if err != nil {
		if path := EventLogPath(sess); path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				return LoadEventLog(sess)
			}
		}
		return nil, err
	}
	if EventLogPath(sess) != "" {
		if err := SaveEventLog(sess, parsed); err != nil {
			return nil, err
		}
	}
	return parsed, nil
}
