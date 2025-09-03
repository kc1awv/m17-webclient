package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSONResponse(t *testing.T) {
	rr := httptest.NewRecorder()
	payload := map[string]string{"status": "ok"}
	if err := writeJSONResponse(rr, payload); err != nil {
		t.Fatalf("writeJSONResponse returned error: %v", err)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	var got map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if got["status"] != "ok" {
		t.Errorf("status = %q, want %q", got["status"], "ok")
	}
}

func TestWriteJSONResponseEncodeError(t *testing.T) {
	rr := httptest.NewRecorder()
	v := map[string]any{"ch": make(chan int)}
	if err := writeJSONResponse(rr, v); err == nil {
		t.Fatalf("writeJSONResponse returned nil error, want non-nil")
	}
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status code = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}
