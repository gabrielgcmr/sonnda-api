<!-- docs/plans/supabase-storage-migration-plan.md -->

# Migração do GCS para Supabase Storage

## Resumo

Migrar para o Supabase Storage o armazenamento compartilhado por:

- documentos permanentes de exames;
- capturas temporárias de 24 horas.

Serão usados dois buckets privados, um cliente HTTP compartilhado e dois adapters vinculados a bucket. Não haverá compatibilidade com `gs://` nem migração de arquivos históricos.

## Etapas de implementação

### 1. Criar os recursos no Supabase

**Status: concluída no código e sincronizada com o Supabase local.**

- Declarar em `supabase/config.toml`:
  - `exam-documents`: privado, PDF, máximo de 5 MiB;
  - `captures`: privado, PDF/JPEG/PNG, máximo de 5 MiB.
- Sincronizar a declaração com `supabase seed buckets --local` no ambiente local e `supabase seed buckets --linked` no projeto remoto correto.
- Não criar migration SQL para os buckets: buckets são dados gerenciados pelo Storage e não fazem parte do schema capturado pelas migrations.
- Não conceder acesso a `anon` ou `authenticated` em `storage.objects`; todo acesso continuará passando pela API.
- Usar uma `SUPABASE_SECRET_KEY` exclusiva por ambiente, apenas no backend.

### 2. Implementar o adapter Supabase

**Status: concluída no código (`internal/infrastructure/filestorage/supabase_storage.go`) e revisada para chaves secret atuais, erros de infraestrutura e respostas HTTP limitadas.**

- Substituir `GCSObjectStorage` por um cliente Supabase Storage baseado em `net/http`, evitando depender do SDK Go comunitário.
- Compartilhar URL, secret key, transporte HTTP e tratamento de erros entre adapters de bucket.
- Implementar:
  - `Upload`, sem upsert;
  - `Open`, retornando streaming autenticado para os futuros fluxos `from-capture`;
  - `Delete`, idempotente para arquivo inexistente;
  - `GetSignedURL`, aceitando `time.Duration`.
- Persistir URIs como `supabase://<bucket>/<object-path>`.
- Validar esquema, bucket e segmentos do caminho antes de qualquer operação.
- Usar nomes opacos:
  - documentos: `patients/{patientID}/exam-documents/{uuid}.pdf`;
  - capturas: `{accountID}/{captureID}.{ext}`, sem nome original.
- Mapear timeout, cancelamento, arquivo ausente e respostas não-2xx para `AppError`, mantendo detalhes sanitizados somente em `Cause`.
- Nunca registrar secret keys ou tokens de URLs assinadas.

### 3. Ajustar contratos e bootstrap

- Alterar os contratos de storage de `documentprocessing` e `capture` para receber `time.Duration` na geração de URL assinada.
- Instanciar no bootstrap:
  - um adapter para `exam-documents`;
  - um adapter para `captures`.
- Injetar imediatamente o primeiro em `ExamsModule`.
- O adapter do bucket `captures` já está injetado em `CaptureModule` desde a etapa 1.3 da feature; a troca do storage de documentos permanentes permanece pendente nesta etapa.
- No futuro endpoint de captura, limitar a URL assinada a `min(5 minutos, expires_at - agora)`.
- No endpoint de documentos permanentes, preservar os 15 minutos atuais.

### 4. Preservar o contrato da nova feature

- Manter o limite de 5 MiB definido nos buckets e na configuração.
- Usar o upload padrão do Supabase para o trecho API → Storage.
- Não adicionar TUS agora: ele não resolveria a retomada do trecho celular → API sem também alterar o protocolo público da captura.
- Manter PDF/JPEG/PNG no bucket temporário e somente PDF no bucket permanente.
- O futuro fluxo “Exames a partir de captura” deverá usar `captures.Open` e criar uma cópia independente por `exam-documents.Upload`.

### 5. Configuração e remoção do GCS

- Adicionar:
  - `SUPABASE_SECRET_KEY`;
  - `SUPABASE_EXAM_DOCUMENTS_BUCKET=exam-documents`;
  - `SUPABASE_CAPTURES_BUCKET=captures`.
- Reutilizar `SUPABASE_PROJECT_URL`.
- Remover `GCS_BUCKET`, construção do cliente GCS, credenciais e volume GCS do Docker Compose.
- Remover a dependência direta `cloud.google.com/go/storage` e executar `go mod tidy`.
- Manter o pacote Document AI e suas dependências Google: ele está inativo e fora do fluxo, mas não deve ser removido como efeito colateral.
- Atualizar README, arquitetura e os planos de captura para substituir as referências a GCS por Supabase Storage.

### 6. Cutover direto

- Antes do deploy, verificar:
  - `exam_documents.storage_uri LIKE 'gs://%'`;
  - `captures.storage_uri LIKE 'gs://%'`.
- Exigir contagem zero ou remoção consciente dos registros descartáveis antes da publicação.
- Sincronizar primeiro os buckets declarados e provisionar os secrets.
- Publicar a API e validar upload, URL assinada, leitura e exclusão.
- Manter o bucket e credenciais GCS durante uma curta janela de rollback; revogá-los depois da validação em produção.

## Interfaces e contratos

- `documentprocessing.FileStorageService` continua responsável por upload, exclusão e URLs assinadas de documentos permanentes.
- `capture.FileStorage` continua responsável por upload, abertura, exclusão e URLs assinadas de capturas temporárias.
- `GetSignedURL` passa a receber `time.Duration` nos dois contratos.
- `storage_uri` continua sendo `TEXT`; não há alteração de schema para as tabelas que armazenam a URI.
- O valor persistido muda de `gs://...` para `supabase://<bucket>/...`.
- Os endpoints existentes de documentos de exame não mudam seu formato de request ou response.

## Testes e critérios de aceite

- Criar testes do adapter com `httptest.Server` cobrindo headers, métodos, escaping, streaming em `Open`, URI retornada e URL assinada relativa.
- Cobrir respostas 401/403/404/409/5xx, timeout, cancelamento, URI inválida, bucket divergente e exclusão repetida.
- Garantir que erros e logs não contenham secret key nem token de URL assinada.
- Testar configurações obrigatórias e inválidas.
- Executar `go test ./...`, `go tool sqlc compile -f internal/infrastructure/database/postgres/sqlc/sqlc.yaml`, migrations locais e `git diff --check`.
- Executar em staging:
  - upload e download de PDF permanente;
  - upload, abertura e exclusão de PDF/JPEG/PNG temporários;
  - negação de acesso direto com chave publishable ou usuário autenticado;
  - indisponibilidade após expiração da URL;
  - confirmação da exclusão física no Storage.

## Assunções e decisões revisadas

- O corte será direto, sem suporte a URIs GCS antigas.
- O limite nos buckets privados e no adapter é de 5 MiB.
- Buckets separados evitam misturar arquivos temporários com documentos clínicos permanentes e permitem restrições de MIME independentes.
- Nenhum endpoint público muda nesta migração, exceto a troca interna do provedor; as futuras rotas de upload de captura continuam no plano próprio da feature.
- Cada ambiente terá um projeto Supabase e uma secret key próprios, usando os mesmos nomes de bucket.
