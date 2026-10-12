// internal/features/documentprocessing/textreading/normalization_test.go
package textreading

import "testing"

func TestNormalizeForSemanticExtractionReplacesPercentConfusables(t *testing.T) {
	rawText := "Hematocrito 43,8 \uFF05\nCHCM 33,7 \uFE6A\nBasofilos 0,0 \u066A\nRDW 12,4 \u332B"
	want := "Hematocrito 43,8 %\nCHCM 33,7 %\nBasofilos 0,0 %\nRDW 12,4 %"

	if got := NormalizeForSemanticExtraction(rawText); got != want {
		t.Fatalf("NormalizeForSemanticExtraction() = %q, want %q", got, want)
	}
}

func TestNormalizeForSemanticExtractionPreservesOtherCharacters(t *testing.T) {
	rawText := "Paciente: Joao - 15,1 g/dL - 1\u2030"
	if got := NormalizeForSemanticExtraction(rawText); got != rawText {
		t.Fatalf("NormalizeForSemanticExtraction() changed unrelated text: %q", got)
	}
}
