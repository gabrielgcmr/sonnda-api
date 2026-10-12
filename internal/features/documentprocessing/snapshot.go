// internal/features/documentprocessing/snapshot.go
package documentprocessing

import (
	"encoding/json"
	"fmt"

	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
)

// Snapshot keeps fields deliberately excluded from the public extraction JSON.
type Snapshot struct {
	Version  int                          `json:"version"`
	Result   labreport.Result             `json:"result"`
	Metadata labreport.ExtractionMetadata `json:"metadata"`
	RawText  *string                      `json:"raw_text"`
	Tests    []TestMetadata               `json:"tests_metadata"`
}

type NodeMetadata struct {
	RawText    *string                       `json:"raw_text"`
	Status     labreport.ExtractionStatus    `json:"status"`
	Confidence *float64                      `json:"confidence"`
	Warnings   []labreport.ExtractionWarning `json:"warnings"`
}

type TestMetadata struct {
	NodeMetadata
	Items []NodeMetadata `json:"items"`
}

// EncodeExtractionSnapshot serializes an extraction result into snapshot JSON.
func EncodeExtractionSnapshot(result *labreport.Result) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("cannot encode nil extraction result")
	}
	s := Snapshot{
		Version:  1,
		Result:   *result,
		Metadata: result.Report.Metadata,
		RawText:  result.Report.RawText,
	}
	for _, test := range result.Report.Tests {
		metadata := TestMetadata{
			NodeMetadata: NodeMetadata{
				RawText:    test.RawText,
				Status:     test.Status,
				Confidence: test.Confidence,
				Warnings:   test.Warnings,
			},
		}
		for _, item := range test.Items {
			metadata.Items = append(metadata.Items, NodeMetadata{
				RawText:    item.RawText,
				Status:     item.Status,
				Confidence: item.Confidence,
				Warnings:   item.Warnings,
			})
		}
		s.Tests = append(s.Tests, metadata)
	}
	return json.Marshal(s)
}

// DecodeExtractionSnapshot deserializes snapshot JSON into an extraction result.
func DecodeExtractionSnapshot(data []byte) (*labreport.Result, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Version != 1 {
		return nil, fmt.Errorf("unsupported extraction snapshot version %d", s.Version)
	}
	if len(s.Tests) != len(s.Result.Report.Tests) {
		return nil, fmt.Errorf("invalid extraction snapshot tests")
	}
	s.Result.Report.Metadata = s.Metadata
	s.Result.Report.RawText = s.RawText
	for i, meta := range s.Tests {
		test := &s.Result.Report.Tests[i]
		test.RawText = meta.RawText
		test.Status = meta.Status
		test.Confidence = meta.Confidence
		test.Warnings = meta.Warnings
		if len(meta.Items) != len(test.Items) {
			return nil, fmt.Errorf("invalid extraction snapshot items")
		}
		for j, m := range meta.Items {
			item := &test.Items[j]
			item.RawText = m.RawText
			item.Status = m.Status
			item.Confidence = m.Confidence
			item.Warnings = m.Warnings
		}
	}
	return &s.Result, nil
}
