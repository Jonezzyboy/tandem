// Package triage asks a local decision model, served by Ollama's systemone
// endpoint, why a CI check failed, so the merge train can rerun failures that
// are not the change's fault.
package triage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Model   string `json:"model"`
	// MinConfidence is how sure a flaky or infra verdict must be to rerun.
	MinConfidence float64 `json:"minConfidence"`
	// Retries caps reruns per leg, so a consistently wrong verdict cannot loop.
	Retries int `json:"retries"`
}

// Normalize fills unset fields: URL from $OLLAMA_HOST, then Ollama's default.
func (c Config) Normalize() Config {
	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	if c.URL == "" {
		c.URL = os.Getenv("OLLAMA_HOST")
	}
	if c.URL == "" {
		c.URL = "http://localhost:11434"
	}
	if !strings.Contains(c.URL, "://") {
		c.URL = "http://" + c.URL
	}
	if c.Model = strings.TrimSpace(c.Model); c.Model == "" {
		c.Model = "nimble"
	}
	if c.MinConfidence <= 0 || c.MinConfidence > 1 {
		c.MinConfidence = 0.85
	}
	if c.Retries <= 0 {
		c.Retries = 1
	}
	return c
}

type Cause string

const (
	Flaky      Cause = "flaky"
	Infra      Cause = "infra"
	Pin        Cause = "pin"
	Regression Cause = "regression"
)

var criteria = map[Cause]string{
	Flaky:      "A nondeterministic test failure that would likely pass on a rerun: timeout, race, order-dependent or time-dependent test",
	Infra:      "A CI environment failure unrelated to the code: runner lost, network or registry error, rate limit, cache or checkout failure",
	Pin:        "The build or tests broke because of the dependency version the head commit just pinned",
	Regression: "The code under test is wrong: compile error, lint failure, or an assertion that reflects real behaviour",
}

type Failure struct {
	Check      string
	HeadCommit string
	Log        string
}

type Verdict struct {
	Cause Cause
	// Confidence is the model's probability for Cause.
	Confidence float64
	// Retry is a flaky or infra cause at or above the configured confidence.
	Retry bool
}

func (v Verdict) String() string {
	return fmt.Sprintf("%s, %.0f%% sure", v.Cause, v.Confidence*100)
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg.Normalize(), http: &http.Client{Timeout: 2 * time.Minute}}
}

// Check confirms the server is up with the model pulled, then classifies a
// sample failure, since only a decision model serves systemone.
func (c *Client) Check(ctx context.Context) (Verdict, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL+"/api/tags", nil)
	if err != nil {
		return Verdict{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Verdict{}, fmt.Errorf("no Ollama server at %s: start it with ollama serve", c.cfg.URL)
	}
	defer resp.Body.Close()
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return Verdict{}, fmt.Errorf("%s did not answer like an Ollama server: %w", c.cfg.URL, err)
	}
	pulled := false
	for _, m := range tags.Models {
		pulled = pulled || m.Name == c.cfg.Model || m.Name == c.cfg.Model+":latest"
	}
	if !pulled {
		return Verdict{}, fmt.Errorf("%s is not pulled: run ollama pull %s", c.cfg.Model, c.cfg.Model)
	}
	return c.Classify(ctx, Failure{
		Check: "test",
		Log:   "--- FAIL: TestRefund (0.00s)\n    refund_test.go:31: got 1050, want 1000\nFAIL",
	})
}

// maxLog keeps the tail of a log, where the failure is, well inside the
// model's context while keeping a cold model's prompt pass short.
const maxLog = 200_000

func (c *Client) Classify(ctx context.Context, f Failure) (Verdict, error) {
	log := f.Log
	if len(log) > maxLog {
		log = log[len(log)-maxLog:]
	}
	body, err := json.Marshal(map[string]any{
		"model": c.cfg.Model,
		"state": map[string]string{"check": f.Check, "head_commit": f.HeadCommit, "failed_log": log},
		"questions": map[string]any{
			"cause": map[string]any{
				"type":         "choice",
				"instructions": "Why did this CI check fail?",
				"criteria":     criteria,
			},
		},
	})
	if err != nil {
		return Verdict{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return Verdict{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Verdict{}, fmt.Errorf("triage model %s: %w", c.cfg.Model, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Verdict{}, fmt.Errorf("triage model %s: %s", c.cfg.Model, resp.Status)
	}
	var out struct {
		Answers struct {
			Cause struct {
				Choice        Cause             `json:"choice"`
				Probabilities map[Cause]float64 `json:"probabilities"`
			} `json:"cause"`
		} `json:"answers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Verdict{}, fmt.Errorf("triage model %s: %w", c.cfg.Model, err)
	}
	a := out.Answers.Cause
	if _, ok := criteria[a.Choice]; !ok {
		return Verdict{}, fmt.Errorf("triage model %s answered %q", c.cfg.Model, a.Choice)
	}
	// The answer's own confidence field runs well below its probability even
	// on clear-cut logs, so the threshold applies to the probability.
	p := a.Probabilities[a.Choice]
	retry := (a.Choice == Flaky || a.Choice == Infra) && p >= c.cfg.MinConfidence
	return Verdict{Cause: a.Choice, Confidence: p, Retry: retry}, nil
}
