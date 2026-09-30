package triage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, choice string, confidence float64, got *map[string]any) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Write([]byte(`{"models":[{"name":"nimble:latest"}]}`))
			return
		}
		if r.URL.Path != "/v1/systemone" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(got); err != nil {
			t.Error(err)
		}
		json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{
			"cause": map[string]any{"type": "choice", "choice": choice, "confidence": 0.1,
				"probabilities": map[string]float64{choice: confidence}},
		}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClassify(t *testing.T) {
	cases := []struct {
		choice     string
		confidence float64
		retry      bool
	}{
		{"flaky", 0.93, true},
		{"infra", 0.85, true},
		{"flaky", 0.6, false},
		{"regression", 0.99, false},
		{"pin", 0.99, false},
	}
	for _, tc := range cases {
		var req map[string]any
		srv := serve(t, tc.choice, tc.confidence, &req)
		c := New(Config{URL: srv.URL, Model: "nimble"})
		v, err := c.Classify(context.Background(), Failure{Check: "test", Log: strings.Repeat("x", maxLog+10) + "FAIL"})
		if err != nil {
			t.Fatal(err)
		}
		if v.Retry != tc.retry || string(v.Cause) != tc.choice {
			t.Errorf("%s@%.2f: got %+v, want retry=%v", tc.choice, tc.confidence, v, tc.retry)
		}
		log := req["state"].(map[string]any)["failed_log"].(string)
		if len(log) != maxLog || !strings.HasSuffix(log, "FAIL") {
			t.Errorf("log sent is %d bytes, want the last %d", len(log), maxLog)
		}
	}
}

func TestClassifyRejectsUnknownChoice(t *testing.T) {
	var req map[string]any
	srv := serve(t, "banana", 0.99, &req)
	if _, err := New(Config{URL: srv.URL}).Classify(context.Background(), Failure{}); err == nil {
		t.Error("want an error for a choice outside the criteria")
	}
}

func TestNormalize(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "127.0.0.1:11500")
	c := Config{}.Normalize()
	if c.URL != "http://127.0.0.1:11500" || c.Model != "nimble" || c.MinConfidence != 0.85 || c.Retries != 1 {
		t.Errorf("defaults = %+v", c)
	}
}

func TestCheck(t *testing.T) {
	var req map[string]any
	srv := serve(t, "regression", 0.99, &req)
	if v, err := New(Config{URL: srv.URL, Model: "nimble"}).Check(context.Background()); err != nil || v.Cause != Regression {
		t.Errorf("check = %+v, %v", v, err)
	}
	if _, err := New(Config{URL: srv.URL, Model: "llama3"}).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "ollama pull llama3") {
		t.Errorf("unpulled model: err = %v", err)
	}
	srv.Close()
	if _, err := New(Config{URL: srv.URL}).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "no Ollama server") {
		t.Errorf("server down: err = %v", err)
	}
}
