// internal/features/documentprocessing/http/temporary_selection_test.go
package http

import "testing"

func TestTemporarySelectionLimits(t *testing.T) {
	for _, test := range []struct {
		kind  string
		count int
		valid bool
	}{
		{"laboratory", 1, true}, {"laboratory", 20, true}, {"laboratory", 21, false},
		{"imaging", 5, true}, {"imaging", 6, false}, {"laboratory", 0, false},
	} {
		err := validateTemporarySelection(test.kind, test.count)
		if (err == nil) != test.valid {
			t.Errorf("kind=%s count=%d error=%v", test.kind, test.count, err)
		}
	}
}
