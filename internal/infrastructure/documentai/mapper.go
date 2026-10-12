// internal/infrastructure/documentai/mapper.go
package documentai

import (
	"strings"

	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"

	"cloud.google.com/go/documentai/apiv1/documentaipb"
)

func extractEntityText(doc *documentaipb.Document, entity *documentaipb.Document_Entity) string {
	if entity.GetMentionText() != "" {
		return entity.GetMentionText()
	}

	textAnchor := entity.GetTextAnchor()
	if textAnchor == nil {
		return ""
	}

	var builder strings.Builder
	fullText := doc.GetText()

	for _, segment := range textAnchor.TextSegments {
		start := segment.GetStartIndex()
		end := segment.GetEndIndex()

		// Validações de segurança
		if int(end) > len(fullText) {
			end = int64(len(fullText))
		}
		if start < 0 || end <= start || int(start) >= len(fullText) {
			continue
		}
		builder.WriteString(fullText[start:end])
	}

	return builder.String()
}

// extractEntityValue retorna o valor normalizado se existir,
// senão cai no texto do anchor/mention já sem espaços extras.
func extractEntityValue(doc *documentaipb.Document, ent *documentaipb.Document_Entity) string {
	if nv := ent.GetNormalizedValue(); nv != nil && nv.GetText() != "" {
		return strings.TrimSpace(nv.GetText())
	}
	return strings.TrimSpace(extractEntityText(doc, ent))
}

func mapDocumentToExtractedLabs(doc *documentaipb.Document) *labreport.ExtractedLabReport {
	out := &labreport.ExtractedLabReport{}

	// Se quiser guardar o texto inteiro do laudo
	if txt := doc.GetText(); txt != "" {
		v := txt
		out.RawText = &v
	}

	for _, ent := range doc.GetEntities() {
		switch ent.GetType() {
		// -------- Cabeçalho simples (1 valor por laudo) --------
		case "patient_name":
			v := extractEntityText(doc, ent)
			out.PatientName = &v

		case "patient_dob":
			v := extractEntityValue(doc, ent)
			out.PatientDOB = &v

		case "lab_name":
			v := extractEntityText(doc, ent)
			out.LabName = &v

		case "lab_phone":
			v := extractEntityText(doc, ent)
			out.LabPhone = &v

		case "insurance_provider":
			v := extractEntityText(doc, ent)
			out.InsuranceProvider = &v

		case "requesting_doctor":
			v := extractEntityText(doc, ent)
			out.RequestingDoctor = &v

		case "technical_manager":
			v := extractEntityText(doc, ent)
			out.TechnicalManager = &v

		case "report_date":
			v := extractEntityValue(doc, ent)
			out.ReportDate = &v

		// O schema original usava test_result/test_item. O schema atual do
		// processor usa panel/observation. Mantemos os dois para que uma troca
		// de versao do processor nao interrompa a extracao de documentos.
		case "test_result", "panel":
			out.Tests = append(out.Tests, mapTestResult(doc, ent))
		}
	}

	return out

}

func mapTestResult(doc *documentaipb.Document, ent *documentaipb.Document_Entity) labreport.ExtractedTestResult {
	var tr labreport.ExtractedTestResult

	for _, prop := range ent.GetProperties() {
		switch prop.GetType() {
		case "test_name":
			tr.TestName = extractEntityText(doc, prop)

		case "material":
			v := extractEntityText(doc, prop)
			tr.Material = &v

		case "method":
			v := extractEntityText(doc, prop)
			tr.Method = &v

		case "collected_at", "collection_date", "collection_datetime", "collected_date":
			v := extractEntityValue(doc, prop)
			tr.CollectedAt = &v

		case "release_at", "released_at", "release_date", "result_date":
			v := extractEntityValue(doc, prop)
			tr.ReleaseAt = &v

		case "test_item", "observation":
			tr.Items = append(tr.Items, mapTestItem(doc, prop))
		}
	}

	return tr
}

func mapTestItem(doc *documentaipb.Document, ent *documentaipb.Document_Entity) labreport.ExtractedTestItem {
	var item labreport.ExtractedTestItem

	for _, prop := range ent.GetProperties() {
		switch prop.GetType() {
		case "parameter_name":
			item.ParameterName = extractEntityText(doc, prop)

		case "result_value", "value_number", "value_text":
			// value_number e value_text sao mutuamente exclusivos no schema novo.
			// Nao sobrescrevemos um valor ja extraido caso o Document AI devolva
			// ambos na mesma observacao.
			if item.ResultValue != nil && strings.TrimSpace(*item.ResultValue) != "" {
				continue
			}
			v := extractEntityText(doc, prop)
			item.ResultValue = &v

		case "unit":
			v := extractEntityText(doc, prop)
			item.ResultUnit = &v

		case "reference_text":
			v := extractEntityText(doc, prop)
			item.ReferenceText = &v
		}
	}

	return item
}
