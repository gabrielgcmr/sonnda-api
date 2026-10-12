// internal/features/documentprocessing/textreading/normalization.go
package textreading

import "strings"

var semanticOCRReplacer = strings.NewReplacer(
	"\uFF05", "%", // FULLWIDTH PERCENT SIGN
	"\uFE6A", "%", // SMALL PERCENT SIGN
	"\u066A", "%", // ARABIC PERCENT SIGN
	"\u332B", "%", // SQUARE PERCENT
)

// NormalizeForSemanticExtraction corrige somente simbolos OCR equivalentes.
func NormalizeForSemanticExtraction(rawText string) string {
	return semanticOCRReplacer.Replace(rawText)
}
