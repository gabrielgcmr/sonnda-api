<!-- docs/adr/ADR-006-revisão-exames-laboratorias.md -->
# ADR-006 — Extração compartilhada e confirmação de exames

Status: implementada. Substitui as decisões de orquestração/persistência anteriores da ADR-005; preserva o extrator semântico e seu contrato.

## Fluxos

- Área de trabalho: PDF com texto selecionável → leitura local → Gemini → resumo copiável. Não grava banco nem armazenamento permanente; o arquivo temporário é removido ao terminar, inclusive em falhas.
- Paciente: verificar acesso → extrair → salvar PDF no bucket privado `exam-documents` do Supabase Storage e rascunho no Postgres → conferir → confirmar no histórico. Não grava resultados clínicos antes da confirmação.
- Terminal: comandos de terminal/CLI para extração foram descontinuados e removidos para evitar abusos; o processamento fica restrito aos fluxos autenticados da API (temporário e rascunho).

O fluxo temporário aceita PDFs de até 10 MiB. O fluxo persistente por paciente aceita PDFs de até 5 MiB, alinhado ao bucket `exam-documents`. O processamento é síncrono, sem fila e sem OCR remoto nos endpoints web. Falhas anteriores à criação do rascunho exigem novo envio.

## Organização e Responsabilidades

A feature `internal/features/documentprocessing` concentra os contratos de processamento:

- `documentprocessing/textextraction`: contrato de leitura (`Extractor`), qualidade (`IsUsableText`) e normalização textual.
- `documentprocessing/labextraction`: contrato (`LabReportTextExtractor`), tipos e schema JSON da extração estruturada.
- `documentprocessing/extraction`: coordenação da extração (`Service`), normalização semântica, avaliação e resumo. Não depende de HTTP, storage nem banco de dados.
- Raiz de `documentprocessing`:
  - `queries.go`: consultas públicas internas de documentos e textos extraídos (`Service`).
  - `drafts.go`: coordenação do upload, criação com snapshot e exclusão de rascunhos.
  - `snapshot.go`: codificação (`EncodeExtractionSnapshot`) e decodificação (`DecodeExtractionSnapshot`) do snapshot versionado.
- Infraestrutura: implementações de leitura de texto (`infrastructure/textextraction`) e cliente Gemini (`infrastructure/gemini`).
- `labdocumentconfirmation`: caso de uso que converte o snapshot conferido em histórico clínico (`patient/exam/laboratory`).

Componentes sem uso foram eliminados: o classificador heurístico (`router.go`), o fallback textual secundário e o helper HTTP `isNilExtractor`.

Normaliza datas e dados antes da conferência, sem converter unidades ou valores; entradas sem identificação são omitidas com aviso. Resultado utilizável contém ao menos um exame e parâmetro identificados com valor, inclusive qualitativo.

Não há classificação automática nem processadores de imagem neste fluxo. A seleção explícita de uma funcionalidade laboratorial define o tipo. PDFs de outros tipos não geram rascunhos sem resultados laboratoriais utilizáveis.

## Persistência

- `exam_documents.status` representa processamento; `review_status` representa `pending`, `confirmed` ou `deleting`.
- `exam_document_extractions` tem relação 1:1 com o documento. Guarda JSON versionado com resultado público e metadados internos que o JSON público omite, incluindo texto e avisos por item. Um trigger impede atualização da fotografia.
- A criação do documento e da fotografia usa uma transação. Se falhar, há tentativa de compensação do upload; falha de compensação permanece na cadeia interna de erro para investigação.
- A confirmação bloqueia o documento e grava laudo, resultados, vínculo, fingerprint e autor/data da confirmação na mesma transação. Repetições retornam o mesmo exame. O Gemini não é chamado na confirmação.
- O histórico laboratorial usa `lab_reports` → `lab_panels` → `observations`; a extração continua usando `tests[]` e `tests[].items[]` no snapshot, com o mapeamento feito somente na confirmação.
- A exclusão marca `deleting`, remove o objeto e depois o registro. Falhas de storage preservam o rascunho para nova tentativa. Objetos já ausentes são tratados como removidos. Documentos confirmados e anteriores não podem ser excluídos por essa operação.
- Documentos anteriores permanecem com `review_status` nulo. Não se atribui confirmação humana retroativa.
- Não há novas gravações em `exam_document_texts`; a consulta dos textos antigos permanece disponível.
- As tabelas documentais têm RLS e acesso direto de `anon`/`authenticated` revogado. O backend verifica acesso ao paciente em cada operação.

## Contratos e compatibilidade

O upload `POST /patients/{patientId}/exam-documents` retorna um rascunho (`201`), nunca um exame automaticamente registrado. O campo `collection_date` foi removido; cada resultado preserva a data encontrada no PDF.

- `GET /exam-documents/{documentId}/extraction`: fotografia para conferência.
- `POST /exam-documents/{documentId}/confirmation`: sem dados clínicos no corpo; retorna o exame confirmado (`200`).
- `DELETE /exam-documents/{documentId}`: descarta rascunho e arquivo (`204`). Uma repetição após exclusão concluída retorna `404`, que o web trata como conclusão.
- Consultas de documentos, PDFs assinados e laudos continuam disponíveis.
- Aliases `/patients/{patientId}/exames` foram removidos.

O web mostra PDF, resumo, dados completos e avisos; não permite edição. Exige conferência explícita antes de confirmar e aceita resultados parciais. Pendências podem ser retomadas em outra sessão. Rascunhos não expiram automaticamente.

**Quebra para o mobile:** a aplicação antiga deixa de registrar resultados ao enviar PDF. Adaptar o mobile para buscar a extração e chamar a confirmação em uma entrega posterior. Não restaurar a gravação automática como compatibilidade.

## Implantação e verificação

1. Aplicar `supabase/migrations/20260930211213_lab_document_review.sql` no ambiente de destino pelo processo de migrations do projeto.
2. Liberar API e web de forma coordenada. Atualizar `artifacts/openapi.json` e executar `task openapi` no consumidor web.
3. Verificar extração temporária, criação/retomada de rascunho, confirmação repetida, exclusão e leitura de exames anteriores.

As migrations são versionadas somente em `supabase/migrations`, incluindo as já aplicadas no ambiente remoto. O sqlc utiliza os arquivos de schema para geração, sem um segundo conjunto de migrations. A migration é aditiva e não remove histórico. O rollback da aplicação não deve reativar uploads automáticos enquanto existirem clientes no novo fluxo.

Validações automatizadas:

```text
go test ./...
go test -tags integration ./internal/features/documentprocessing/postgres ./internal/features/patient/exam/laboratory/postgres
go tool sqlc compile -f internal/infrastructure/persistence/postgres/sqlc/sqlc.yaml
make openapi-export
task openapi
bun run test
bun run lint
bun run build
```

Integrações exigem `LABS_TEST_DATABASE_URL` apontando para Postgres local isolado. Os testes criam schemas temporários; o teste da migration cria, se necessário, os papéis locais `anon`, `authenticated` e `service_role`.

Monitorar erros de extração, confirmação e compensação pelo logging centralizado. Não registrar laudos, resultados ou conteúdo de PDFs nos logs.
