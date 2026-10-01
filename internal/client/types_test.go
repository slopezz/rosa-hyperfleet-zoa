package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFlexString_WhenUnmarshalString_ItShouldStoreValue(t *testing.T) {
	input := `"hello world"`
	var f FlexString
	if err := json.Unmarshal([]byte(input), &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.String() != "hello world" {
		t.Errorf("expected 'hello world', got %q", f.String())
	}
}

func TestFlexString_WhenUnmarshalArray_ItShouldStoreRawJSON(t *testing.T) {
	input := `[{"name":"pod-1"},{"name":"pod-2"}]`
	var f FlexString
	if err := json.Unmarshal([]byte(input), &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.String() != input {
		t.Errorf("expected raw JSON array, got %q", f.String())
	}
}

func TestFlexString_WhenUnmarshalObject_ItShouldStoreRawJSON(t *testing.T) {
	input := `{"key":"value","nested":{"a":1}}`
	var f FlexString
	if err := json.Unmarshal([]byte(input), &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.String() != input {
		t.Errorf("expected raw JSON object, got %q", f.String())
	}
}

func TestFlexString_WhenUnmarshalNull_ItShouldStoreEmpty(t *testing.T) {
	input := `null`
	var f FlexString
	if err := json.Unmarshal([]byte(input), &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.String() != "" {
		t.Errorf("expected empty, got %q", f.String())
	}
}

func TestFlexString_WhenMarshalEmpty_ItShouldReturnNull(t *testing.T) {
	f := FlexString("")
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("expected 'null', got %s", data)
	}
}

func TestFlexString_WhenMarshalValidJSON_ItShouldReturnRawJSON(t *testing.T) {
	f := FlexString(`[{"name":"pod-1"}]`)
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `[{"name":"pod-1"}]` {
		t.Errorf("expected raw JSON, got %s", data)
	}
}

func TestFlexString_WhenMarshalPlainText_ItShouldQuoteAsString(t *testing.T) {
	f := FlexString("plain text output")
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `"plain text output"` {
		t.Errorf("expected quoted string, got %s", data)
	}
}

func TestFlexString_WhenMarshalJSONObject_ItShouldReturnRawJSON(t *testing.T) {
	f := FlexString(`{"key":"value"}`)
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"key":"value"}` {
		t.Errorf("expected raw JSON object, got %s", data)
	}
}

func TestAPIError_WhenReasonAndHTTPStatus_ItShouldIncludeHTTP(t *testing.T) {
	e := &APIError{Code: "not_found", Reason: "execution not found", Surface: APISurfaceAPI, HTTPStatus: 404}
	if e.Error() != "ZOA API (HTTP 404): execution not found" {
		t.Errorf("unexpected: %q", e.Error())
	}
}

func TestAPIError_WhenReasonSet_ItShouldReturnReason(t *testing.T) {
	e := &APIError{Code: "not_found", Reason: "execution not found", Message: "some message"}
	if e.Error() != "execution not found" {
		t.Errorf("expected 'execution not found', got %q", e.Error())
	}
}

func TestAPIError_WhenOnlyMessage_ItShouldReturnMessage(t *testing.T) {
	e := &APIError{Code: "invalid", Message: "missing parameter"}
	if e.Error() != "missing parameter" {
		t.Errorf("expected 'missing parameter', got %q", e.Error())
	}
}

func TestAPIError_WhenOnlyCode_ItShouldReturnCode(t *testing.T) {
	e := &APIError{Code: "forbidden"}
	if e.Error() != "forbidden" {
		t.Errorf("expected 'forbidden', got %q", e.Error())
	}
}

func TestLambdaRuntimeError_WhenInvalidEntrypoint_ItShouldReturnUnavailableMessage(t *testing.T) {
	e := &LambdaRuntimeError{
		ErrorType:    "Runtime.InvalidEntrypoint",
		ErrorMessage: "RequestId: abc",
		Surface:      APISurfaceAccess,
		HTTPStatus:   200,
	}
	msg := e.Error()
	if strings.Count(msg, "ZOA Access API") != 1 {
		t.Errorf("expected surface once, got %q", msg)
	}
	if !strings.Contains(msg, "ZOA Access API (HTTP 200):") {
		t.Errorf("expected HTTP status in message, got %q", msg)
	}
	if !strings.Contains(msg, "Runtime.InvalidEntrypoint") {
		t.Errorf("expected aws error type in message, got %q", msg)
	}
	if !strings.Contains(msg, "RequestId: abc") {
		t.Errorf("expected aws errorMessage in message, got %q", msg)
	}
}

func TestLambdaRuntimeError_WhenExitError_ItShouldReturnUnavailableMessage(t *testing.T) {
	e := &LambdaRuntimeError{ErrorType: "Runtime.ExitError", ErrorMessage: "exit status 1", HTTPStatus: 502}
	msg := e.Error()
	if !strings.Contains(msg, "HTTP 502") {
		t.Errorf("expected HTTP 502, got %q", msg)
	}
	if !strings.Contains(msg, "exit status 1") {
		t.Errorf("expected aws detail, got %q", msg)
	}
	if !e.IsUnavailable() {
		t.Error("expected IsUnavailable() true")
	}
}

func TestLambdaRuntimeError_WhenDeadlineExceeded_ItShouldReturnTimeoutMessage(t *testing.T) {
	e := &LambdaRuntimeError{ErrorType: "Runtime.DeadlineExceeded", HTTPStatus: 503}
	if !strings.Contains(e.Error(), "HTTP 503") {
		t.Errorf("unexpected: %q", e.Error())
	}
	if e.IsUnavailable() {
		t.Error("expected IsUnavailable() false for deadline exceeded")
	}
}

func TestLambdaRuntimeError_WhenOtherErrorWithMessage_ItShouldIncludeBoth(t *testing.T) {
	e := &LambdaRuntimeError{ErrorType: "Runtime.Unknown", ErrorMessage: "something broke"}
	if e.Error() != "aws Runtime.Unknown: something broke" {
		t.Errorf("unexpected: %q", e.Error())
	}
}

func TestLambdaRuntimeError_WhenOtherErrorNoMessage_ItShouldShowType(t *testing.T) {
	e := &LambdaRuntimeError{ErrorType: "Runtime.Unknown"}
	if e.Error() != "aws Runtime.Unknown" {
		t.Errorf("unexpected: %q", e.Error())
	}
}

func TestAuditEntry_WhenShortPath_ItShouldStripPrefix(t *testing.T) {
	e := AuditEntry{Path: "/api/v0/trusted-actions/get_pods/run"}
	if e.ShortPath() != "get_pods/run" {
		t.Errorf("expected 'get_pods/run', got %q", e.ShortPath())
	}
}

func TestAuditEntry_WhenShortPathNoPrefix_ItShouldReturnFull(t *testing.T) {
	e := AuditEntry{Path: "/version"}
	if e.ShortPath() != "/version" {
		t.Errorf("expected '/version', got %q", e.ShortPath())
	}
}
