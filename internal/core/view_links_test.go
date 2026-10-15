package core

import (
	"slices"
	"testing"

	"github.com/jonezzyboy/tandem/internal/gh"
)

func TestNewPRViewLinksFailedChecks(t *testing.T) {
	pr := &gh.PR{Number: 7, State: "OPEN", StatusCheckRollup: []gh.Check{
		{Name: "test", Status: "COMPLETED", Conclusion: "FAILURE", DetailsURL: "https://github.com/acme/api/actions/runs/11/job/22"},
		{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Context: "ci/jenkins", State: "FAILURE", DetailsURL: "https://jenkins.example/job/9"},
	}}
	got := NewPRView(pr).FailedChecks
	want := []CheckLink{
		{Name: "test", URL: "https://github.com/acme/api/actions/runs/11/job/22", Rerunnable: true},
		{Name: "ci/jenkins", URL: "https://jenkins.example/job/9"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("FailedChecks = %+v, want %+v", got, want)
	}
}
