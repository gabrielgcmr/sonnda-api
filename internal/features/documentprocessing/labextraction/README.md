<!-- internal/features/documentprocessing/labextraction/README.md -->
# Contrato de extração laboratorial

Este pacote define o contrato de entrada textual ou multimodal (`ExtractLabReportInput`), o contrato do extrator semântico (`LabReportExtractor`), os tipos de dados e o JSON Schema versionado (`lab_report.schema.json`).

## Entrada e responsabilidades

`ExtractLabReportInput` aceita exatamente uma origem: texto não vazio ou mídia PDF, JPEG ou PNG.
`LabReportExtractor` é a interface implementada por adaptadores externos (como Gemini em `internal/infrastructure/gemini`).
Paciente, usuário, persistência, rascunhos e storage ficam a cargo da feature `documentprocessing` e dos casos de uso de confirmação.

## Correspondência com o banco

| JSON                                                                          | Destino existente                        |
| ----------------------------------------------------------------------------- | ---------------------------------------- |
| `patient_name`, `patient_dob`, `lab_name`, `lab_phone`                        | Campos correspondentes em `lab_reports`  |
| `insurance_provider`, `requesting_doctor`, `technical_manager`, `report_date` | Campos correspondentes em `lab_reports`  |
| `tests[]`                                                                     | Registros em `lab_panels`                |
| `test_name`, `material`, `method`, `collected_at`, `release_at`               | Campos correspondentes em `lab_panels`   |
| `tests[].items[]`                                                             | Registros em `observations`              |
| `parameter_name`, `result_value`, `result_unit`, `reference_text`             | Campos correspondentes em `observations` |

O schema acompanha os campos de extração usados pelo mapper de confirmação. Não é um dump
das tabelas: IDs, chaves estrangeiras, fingerprint, timestamps de gravação, texto
bruto e metadados técnicos não são gerados pelo modelo.
`RawText` e os metadados continuam existindo no DTO para preenchimento pelo backend,
mas são excluídos do JSON de resposta da LLM.

## Regras do JSON

- Arquivo: `lab_report.schema.json`, acessível em Go por `LabReportResponseSchema()`.
- Versão: `LabReportSchemaVersion`. Mudanças incompatíveis exigem revisar a versão.
- As chaves declaradas são obrigatórias; campos opcionais clinicamente usam `null`.
- `result_value` é texto, como no banco: `14,2`, `245.000`, `< 5`, `Negativo`.
- Não converter unidades, preencher valores ausentes ou criar nomes canônicos.
- Referências permanecem compatíveis, mas podem ser `null`.
- Datas seguem as representações descritas no schema e aceitas pelo mapper.
- Percentual e absoluto são itens distintos, preservando nome, valor e unidade.
- `tests: []` representa ausência de resultados. Um pedido ou atestado sem resultados
  não deve gerar itens somente por mencionar analitos, datas ou durações.

## Exemplos e fixtures

`testdata/` contém exemplos sintéticos versionados. Cada caso tem:

- `<nome>.input.txt`: texto de entrada;
- `<nome>.expected.json`: saída esperada, revisada manualmente;
- entrada em `cases.json`: nome e indicação de presença de resultados estruturados.

Os testes validam o schema e a compatibilidade do JSON com os DTOs.

```powershell
go test ./internal/features/documentprocessing/labextraction -v
```

O validador JSON Schema usado pelos testes é `github.com/santhosh-tekuri/jsonschema/v6`.
A validação do schema funciona localmente, sem API key, OCR ou banco de dados.
