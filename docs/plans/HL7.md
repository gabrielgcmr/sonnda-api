Consolidei as evidências com linhas no repositório e já tenho o desenho alvo em FHIR R5 com migração incremental sem breaking change. Vou te entregar agora o gap campo a campo, arquitetura de transição e um plano por fases com critérios de aceite.

Perfeito. Com base no estado atual, o caminho mais seguro é uma migração em camadas: manter o contrato atual para consumidores legados e introduzir uma camada FHIR R5 canônica em paralelo.

Parecer objetivo
Seu diagnóstico está correto: hoje a modelagem de exames é proprietária e não aderente a HL7 FHIR R5 no nível de semântica clínica exigido para DiagnosticReport e Observation.

Gap-to-target técnico (campo a campo)

Header do laudo (atual: lab_reports) → FHIR DiagnosticReport
id → DiagnosticReport.id
Gap: OK estruturalmente (UUID local), falta estratégia de id canônico FHIR.
patient_id → DiagnosticReport.subject (Reference Patient/{id})
Gap: precisa serializar como Reference FHIR.
report_date → DiagnosticReport.effectiveDateTime ou issued
Gap: precisa regra clara de mapeamento entre data clínica e data de emissão.
lab_name, lab_phone, technical_manager → performer / resultsInterpreter
Gap: hoje texto livre; em FHIR idealmente Reference Organization/Practitioner.
raw_text → presentedForm.data (base64) ou extensão
Gap: não modelado como Attachment FHIR.
fingerprint → identifier (system/value) para deduplicação
Gap: dedupe existe, mas não como Identifier semântico.
Campos críticos ausentes:
DiagnosticReport.status (obrigatório)
DiagnosticReport.code (obrigatório semântico)
category (recomendado para laboratoriais)
result[] (references para Observation)
Resultado de teste (atual: lab_results) → Observation “painel” ou DiagnosticReport.result
test_name → Observation.code.text (mínimo) ou coding LOINC
Gap: sem code.coding padronizado.
material → Observation.specimen (Reference) ou extension
Gap: sem recurso Specimen ou codificação.
method → Observation.method (CodeableConcept)
Gap: texto livre, sem terminologia.
collected_at → Observation.effectiveDateTime
Gap: parcial, precisa normalização de timezone e uso clínico consistente.
release_at → Observation.issued
Gap: parcial, sem contrato explícito FHIR.
Campos críticos ausentes:
Observation.status (obrigatório)
Observation.category (vital para laboratório)
subject/reference padrão
interpretation, note, basedOn, encounter (quando aplicável)
Item de resultado (atual: lab_result_items) → Observation principal ou Observation.component
parameter_name → Observation.code.text/coding
Gap: sem LOINC/local code system.
result_value + result_unit → Observation.valueQuantity
Gap: hoje texto livre; falta value, unit, system, code (UCUM).
reference_text → Observation.referenceRange textual
Gap: sem low/high tipados; só texto.
Ausências comuns:
value[x] tipado (Quantity, CodeableConcept, string, boolean)
referenceRange.low/high com unidade
interpretation (ex: H, L, N)
Contrato HTTP/OpenAPI
Atual: array/object genéricos
Gap: sem schema FHIR robusto para validação e interoperabilidade.
Rotas atuais podem permanecer, mas falta exposição REST FHIR:
GET /fhir/R5/DiagnosticReport
GET /fhir/R5/DiagnosticReport/{id}
GET /fhir/R5/Observation/{id}
Pesquisa por patient, date, code, status
Target recomendado (sem quebrar consumidores atuais)

Arquitetura de coexistência:

Canonical model FHIR R5 no backend
Adaptador Legacy Out para manter payload atual
Novos endpoints FHIR para novos clientes
Ingestão OCR passa a preencher:
Campos estruturados FHIR quando possível
Campos “text only” com DataAbsentReason/extensões quando não possível
Desenho de banco (migração incremental)

Opção recomendada: manter tabelas atuais e adicionar tabelas canônicas FHIR-like (não precisa ser 1:1 JSON puro, pode ser híbrido relacional + JSONB).

Novas tabelas mínimas:

fhir_diagnostic_reports
id UUID PK
legacy_lab_report_id UUID NULL FK lab_reports(id)
patient_id UUID NOT NULL
status TEXT NOT NULL
code_system TEXT NULL
code_value TEXT NULL
code_text TEXT NOT NULL
effective_at TIMESTAMPTZ NULL
issued_at TIMESTAMPTZ NULL
performer_ref TEXT[] NULL
identifier_system TEXT NULL
identifier_value TEXT NULL
raw_resource JSONB NOT NULL
created_at, updated_at
fhir_observations
id UUID PK
diagnostic_report_id UUID NULL FK fhir_diagnostic_reports(id)
legacy_lab_result_id UUID NULL FK lab_results(id)
legacy_lab_result_item_id UUID NULL FK lab_result_items(id)
patient_id UUID NOT NULL
status TEXT NOT NULL
category_code TEXT NOT NULL default laboratory
code_system TEXT NULL
code_value TEXT NULL
code_text TEXT NOT NULL
effective_at TIMESTAMPTZ NULL
issued_at TIMESTAMPTZ NULL
value_type TEXT NULL
value_num NUMERIC NULL
value_text TEXT NULL
unit_text TEXT NULL
unit_ucum_code TEXT NULL
unit_ucum_system TEXT NULL default http://unitsofmeasure.org
ref_low NUMERIC NULL
ref_high NUMERIC NULL
ref_text TEXT NULL
interpretation_code TEXT NULL
raw_resource JSONB NOT NULL
created_at, updated_at
terminology_map_lab
source_kind TEXT NOT NULL (test_name, parameter_name, unit)
source_value TEXT NOT NULL
target_system TEXT NOT NULL
target_code TEXT NOT NULL
target_display TEXT NULL
confidence NUMERIC NULL
active BOOLEAN NOT NULL default true
UNIQUE(source_kind, source_value)
Índices críticos:

fhir_diagnostic_reports(patient_id, effective_at desc)
fhir_observations(patient_id, effective_at desc)
fhir_observations(code_system, code_value)
fhir_observations(status)
GIN em raw_resource se houver pesquisa por JSON
DTO e camada de aplicação

Entrada OCR (mantida):

Continua usando DTO atual:
lab_dto.go:23
Adicionar DTOs internos canônicos:

DiagnosticReportCanonicalDTO
ObservationCanonicalDTO
TerminologyCodingDTO
QuantityDTO
ReferenceRangeDTO
Pipeline novo:

Extract
Normalize (trim, timezone, número decimal)
Terminology resolution (LOINC/SNOMED/local)
Build FHIR canonical DTOs
Persist canonical
Persist legado (durante coexistência) ou gerar legado via projection
Publicar evento lab_report.mapped_to_fhir com status de mapeamento
Status de qualidade de mapeamento por item:

mapped_exact
mapped_text_only
unmapped
Isso ajuda governança e backlog de terminologia.
Endpoints sem quebra

Manter legados:

GET /v1/patients/{id}/labs
POST /v1/patients/{id}/labs
Conforme rotas atuais em routes.go:91
Adicionar novos:

GET /fhir/R5/DiagnosticReport?patient={id}&date=ge2026-01-01
GET /fhir/R5/DiagnosticReport/{id}
GET /fhir/R5/Observation?patient={id}&code={system|code}
GET /fhir/R5/Observation/{id}
Opcional: GET /fhir/R5/metadata (CapabilityStatement)
Compatibilidade:

POST legado continua igual na resposta.
Internamente já cria também recursos canônicos.
GET legado inicialmente lê das tabelas antigas.
Fase posterior: GET legado pode ler de projection gerada do canônico.
Plano de migração por fases

Fase 0: preparação

Adicionar tabelas fhir_* e terminology_map_lab.
Sem mudança em contrato externo.
Fase 1: dual-write seguro

No use case de upload, escrever legado + canônico na mesma transação (ou outbox com retry).
Persistir raw_resource JSONB para rastreabilidade.
Fase 2: leitura FHIR

Expor endpoints /fhir/R5.
Publicar OpenAPI/IG interna dessas rotas.
Fase 3: retrocarga

Backfill dos lab_reports históricos para fhir_*.
Marcar qualidade de mapeamento por observação.
Fase 4: hardening

Métricas de cobertura terminológica.
Alertas para taxa de unmapped acima de limiar.
Testes de contrato e regressão.
Fase 5: convergência

Opcionalmente tornar canônico a fonte primária de leitura.
Manter endpoint legado como adapter enquanto houver consumidores.
Critérios de aceite técnicos

100% dos uploads novos geram DiagnosticReport.status e Observation.status válidos.
= 95% dos itens numéricos com unidade mapeados para valueQuantity + UCUM.

Endpoint legado sem alteração de payload e sem aumento de erro 4xx/5xx.
Backfill idempotente e auditável.
Validação automática de recursos FHIR em testes de integração.
Riscos e mitigação

Risco: baixa cobertura LOINC/UCUM no início.
Mitigação: code_text obrigatório + mapping table incremental + quality flags.
Risco: divergência entre legado e canônico no dual-write.
Mitigação: transação única ou outbox; reconciliador diário.
Risco: performance em buscas FHIR.
Mitigação: índices por patient/date/code e paginação estrita.
Assunções e perguntas abertas

Assumi que vocês não precisam, neste momento, suporte completo a transações FHIR Bundle.
Assumi que Patient já possui identificador estável reutilizável como Reference.
Pergunta 1: vocês querem endpoint FHIR apenas de leitura agora, ou também criação via POST DiagnosticReport?
Pergunta 2: preferem canônico relacional híbrido (colunas + JSONB) ou JSONB-first com projeções?
Pergunta 3: existe lista inicial de exames prioritários para mapeamento LOINC (top 20)?
Se você quiser, no próximo passo eu já te entrego um blueprint implementável com:

DDL inicial de migração SQL,
contratos de DTO canônico em Go,
assinatura dos novos handlers e queries de busca FHIR,
matriz de mapeamento inicial (hemograma, glicose, creatinina, ureia, TSH).