package integration

import (
	"encoding/json"
	"io"
	"testing"
)

func TestBuildHTTPRequest_BodyPath(t *testing.T) {
	action := &Action{
		Method:     "POST",
		Endpoint:   "https://api.x.com/2/tweets",
		AuthHeader: "Bearer {{token}}",
		BodyFormat: "json",
		Params: []Param{
			{Name: "text", Required: true},
			{Name: "reply_to", Required: false, BodyPath: "reply.in_reply_to_tweet_id"},
		},
	}
	cred := &Credential{AccessToken: "test-token"}
	params := map[string]string{
		"text":     "Hello world",
		"reply_to": "123456789",
	}

	req, err := buildHTTPRequest(action, cred, params, nil)
	if err != nil {
		t.Fatalf("buildHTTPRequest failed: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshaling body: %v", err)
	}

	// "text" should be at the top level (no body_path)
	if result["text"] != "Hello world" {
		t.Errorf("expected text='Hello world', got %v", result["text"])
	}

	// "reply_to" should NOT be at the top level
	if _, ok := result["reply_to"]; ok {
		t.Error("reply_to should not be a top-level key when body_path is set")
	}

	// "reply.in_reply_to_tweet_id" should be nested
	reply, ok := result["reply"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'reply' to be a nested object, got %T: %v", result["reply"], result["reply"])
	}
	if reply["in_reply_to_tweet_id"] != "123456789" {
		t.Errorf("expected reply.in_reply_to_tweet_id='123456789', got %v", reply["in_reply_to_tweet_id"])
	}
}

func TestBuildHTTPRequest_BodyPathWithoutValue(t *testing.T) {
	action := &Action{
		Method:     "POST",
		Endpoint:   "https://api.x.com/2/tweets",
		AuthHeader: "Bearer {{token}}",
		BodyFormat: "json",
		Params: []Param{
			{Name: "text", Required: true},
			{Name: "reply_to", Required: false, BodyPath: "reply.in_reply_to_tweet_id"},
		},
	}
	cred := &Credential{AccessToken: "test-token"}
	params := map[string]string{
		"text": "Hello world",
	}

	req, err := buildHTTPRequest(action, cred, params, nil)
	if err != nil {
		t.Fatalf("buildHTTPRequest failed: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshaling body: %v", err)
	}

	// Only "text" should be present
	if result["text"] != "Hello world" {
		t.Errorf("expected text='Hello world', got %v", result["text"])
	}

	// "reply" should not exist when reply_to param is not provided
	if _, ok := result["reply"]; ok {
		t.Error("reply object should not exist when reply_to param is not provided")
	}
}
