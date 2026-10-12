// internal/features/documentprocessing/textreading/quality_test.go
package textreading

import "testing"

func TestIsUsableText_AcceptsClinicalText(t *testing.T) {
	text := "Ressonância magnética do joelho. Técnica do exame preservada. Achados compatíveis com lesão meniscal. Conclusão ao final do laudo."

	if !IsUsableText(text) {
		t.Fatal("expected clinical text to be usable")
	}
}

func TestIsUsableText_AcceptsAccentedLabText(t *testing.T) {
	text := "Registro do laboratório no conselho. Hemograma completo com hemácias, leucócitos e plaquetas. Material sangue total coletado em tubo EDTA."

	if !IsUsableText(text) {
		t.Fatal("expected accented lab text to be usable")
	}
}

func TestIsUsableText_RejectsShortText(t *testing.T) {
	if IsUsableText("tomografia") {
		t.Fatal("expected short text to be rejected")
	}
}

func TestIsUsableText_RejectsTextWithoutClinicalSignals(t *testing.T) {
	text := "Documento recebido em formato digital com informacoes gerais sem conteudo clinico suficiente para classificacao automatica."

	if IsUsableText(text) {
		t.Fatal("expected text without clinical signals to be rejected")
	}
}
