// internal/features/documentprocessing/textreading/quality.go
package textreading

import "strings"

func IsUsableText(text string) bool {
	normalized := normalizeText(text)
	if len([]rune(normalized)) < 80 {
		return false
	}

	matches := 0
	for _, signal := range clinicalSignals {
		if strings.Contains(normalized, signal) {
			matches++
		}
	}

	return matches >= 1
}

var clinicalSignals = []string{
	"achados",
	"conclusao",
	"impressao diagnostica",
	"tomografia",
	"ressonancia",
	"ultrassonografia",
	"radiografia",
	"exame",
	"laudo",
	"mamografia",
	"biopsia",
	"patologia",
	"citologia",
	"hematologia",
	"hemograma",
	"glicose",
	"glicemia",
	"plaquetas",
	"leucocitos",
	"laboratorio",
	"microbiologia",
	"genetica",
	"tamanho",
	"volume",
}

func normalizeText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c",
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c",
		"\n", " ", "\r", " ", "\t", " ",
	)
	text = replacer.Replace(text)
	return strings.Join(strings.Fields(text), " ")
}
