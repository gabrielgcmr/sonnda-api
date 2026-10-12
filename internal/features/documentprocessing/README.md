<!-- internal/features/documentprocessing/README.md -->
# Document Processing

Feature responsável pelo processamento de documentos clínicos e laudos laboratoriais, englobando leitura textual, extração estruturada semântica, gerenciamento de rascunhos e fotografia persistida (snapshot).

## Estrutura de Pacotes

```text
internal/features/documentprocessing/
├── authorization.go        # Autoriza operações standalone e vinculadas a paciente/documento
├── drafts.go               # Criação, leitura e exclusão de rascunhos
├── dto.go                  # DTOs de saída dos documentos e textos extraídos
├── error.go                # Tradução de erros de persistência para AppError
├── file_storage.go         # Contrato de armazenamento de arquivos
├── policy.go               # Ações e regras de autorização desta feature
├── queries.go              # Consultas de documentos e textos extraídos
├── repository.go           # Contrato de consulta de documentos
├── service.go              # Interface pública interna para consultas
├── snapshot.go             # Codificação e decodificação do snapshot da extração
├── standalone_lab.go       # Coordenação da extração laboratorial temporária
├── standalone_imaging_report.go # Coordenação da extração temporária de laudos de imagem
├── domain/                 # Modelos e regras dos documentos e textos extraídos
├── http/                   # Handlers Huma para documentos, revisão e extração temporária
├── labreport/              # Contrato, schema e coordenação de laudos laboratoriais
├── imagingreport/          # Contrato, schema e coordenação de laudos escritos de imagem
├── postgres/               # Adaptadores PostgreSQL para documentos e rascunhos
└── textreading/            # Contrato e regras de leitura, normalização e qualidade do texto
```

Os testes ficam junto aos pacotes, em arquivos `*_test.go`; os testes PostgreSQL
que exigem banco estão identificados como testes de integração.

## Direção das Dependências

1. **`domain`**:
   - Contém os modelos e regras de domínio de documentos, textos extraídos,
     metadados laboratoriais e revisão.
   - Não depende de HTTP, persistência ou da raiz da feature.
2. **`textreading` (Contrato compartilhado)**:
   - Define a interface de leitura, a normalização e a avaliação de qualidade do texto.
   - Não depende da raiz da feature, de HTTP, banco de dados ou SDKs de terceiros.
3. **`labreport` e `imagingreport` (Processamento especializado)**:
   - Cada pacote reúne contrato, schema, DTOs e coordenação do seu tipo de laudo.
   - Compõem `textreading.Extractor` com o extrator semântico correspondente.
   - Executam normalização de entrada, avaliação e geração de resumo.
   - Não depende de HTTP, banco de dados, storage nem da raiz de `documentprocessing`.
4. **Raiz de `documentprocessing`**:
   - Mantém contratos, DTOs, consultas e coordenação dos fluxos de documentos.
   - `authorization.go` e `policy.go` verificam acesso ao paciente e regras por ação; não substituem a autorização compartilhada de `authz`.
   - `drafts.go` coordena extração, upload no storage e persistência do rascunho com snapshot. Os arquivos `standalone_*.go` coordenam extrações temporárias sem persistência.
   - `snapshot.go` (`EncodeExtractionSnapshot` / `DecodeExtractionSnapshot`) serializa `labreport.Result` preservando metadados privados (texto bruto, status, confiança e avisos por item).
5. **Adapters HTTP e PostgreSQL**:
   - `http` registra as rotas Huma e traduz erros para respostas HTTP.
   - `postgres` implementa os contratos de persistência da feature.
   - Ambos dependem dos contratos e tipos necessários da feature; a raiz não depende desses adapters.
6. **Infraestrutura e casos de uso externos**:
   - A leitura de PDFs com `pdftotext` vive em `internal/infrastructure/pdftotext`; o OCR com Tesseract fica isolado em `internal/infrastructure/ocr` para uso futuro. Os extratores Gemini vivem em `internal/infrastructure/gemini`.
   - A conversão do snapshot em histórico clínico é responsabilidade do caso de uso `internal/application/usecase/labdocumentconfirmation`.

## Consumidores da Extração

O pipeline de extração atende aos fluxos temporários de laudos laboratoriais e de imagem, além do fluxo persistido de exames laboratoriais:

1. **Extração temporária (`StandaloneLabExtractionHandler`)**:
   - Rota `POST /lab-extractions`.
   - Utilizada para conferência imediata em área de trabalho.
   - O PDF temporário é descartado após o processamento, inclusive em falhas. Não grava em banco nem em storage persistente.
2. **Criação de rascunho (`Drafts`)**:
   - Rota `POST /patients/{patientId}/exam-documents`.
   - Grava o PDF original no storage, persiste o snapshot da extração e gera um documento com status `pending`.
   - Permite conferência posterior e confirmação transacional no histórico do paciente.
3. **Extração temporária de laudo de imagem (`StandaloneImagingExtractionHandler`)**:
   - Rota `POST /imaging-report-extractions` e variante `/from-capture`.
   - Aceita laudos escritos em PDF, JPEG ou PNG sem persistir o documento.

> **Nota:** Utilitários de extração via terminal/CLI foram descontinuados e removidos para restringir a execução exclusivamente às rotas autenticadas da API.
