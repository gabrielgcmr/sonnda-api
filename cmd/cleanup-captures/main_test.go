// cmd/cleanup-captures/main_test.go
package main

import (
	"errors"
	"testing"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
)

func TestCleanupExitCode(t *testing.T) {
	tests := []struct {
		name   string
		report *capture.CleanupReport
		err    error
		want   int
	}{
		{name: "success", report: &capture.CleanupReport{}, want: exitSuccess},
		{name: "partial failure", report: &capture.CleanupReport{Errors: []error{errors.New("storage unavailable")}}, want: exitFailure},
		{name: "fatal failure", report: &capture.CleanupReport{}, err: errors.New("database unavailable"), want: exitFailure},
		{name: "missing report", want: exitFailure},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cleanupExitCode(test.report, test.err); got != test.want {
				t.Fatalf("cleanupExitCode() = %d, want %d", got, test.want)
			}
		})
	}
}
