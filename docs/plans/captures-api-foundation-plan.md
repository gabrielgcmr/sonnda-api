<!-- docs/plans/captures-api-foundation-plan.md -->

# Capturas — etapa 1: fundação na API

## Objetivo e limites

Entregar, em subetapas revisáveis, o pareamento por QR, o envio pelo celular sem login e uma caixa de entrada temporária por conta. Este plano detalha a etapa 1 de `sonnda-svelte/docs/plans/captures-implementation-plan.md`. A captura não recebe paciente, categoria ou resultado clínico. Extração avulsa e criação de rascunho de exame a partir de uma captura pertencem às etapas 3 e 4 do plano web.

Decisões já tomadas: o QR autoriza o celular sem login; a credencial de envio dura no máximo **12 horas**; o computador precisa manter presença recente para aceitar uploads; cada captura fica disponível por **24 horas**, mesmo após a desconexão e após seu uso em um destino. Aceitar PDF, JPEG e PNG de até **5 MiB** por arquivo.

## Arquitetura

- Criar `internal/features/capture` como feature própria, com modelos e regras de sessão/captura, contratos de repositório e armazenamento, adaptador PostgreSQL e handlers HTTP. Ela não importa `documentprocessing` nem conhece paciente ou tipo clínico.
- Compor a feature em `internal/application/bootstrap` e registrar suas rotas em `internal/api`: operações do computador no grupo com Supabase Bearer e onboarding; reivindicação do QR fora desse grupo; operações do celular com middleware exclusivo para a credencial de captura. O adapter Supabase vinculado ao bucket privado `captures` implementa o contrato de armazenamento da feature.
- Nas etapas posteriores, casos de uso de integração obtêm a captura por `captureId` e conta autorizada e entregam o arquivo ao processamento do destino. A feature `capture` continua responsável apenas pelo arquivo temporário e sua expiração; o destino cria sua própria cópia persistente quando necessário.

## Subetapas

### 1.1 — Modelo, migration e contratos internos

**Status: concluída em código e validada em PostgreSQL local.**

- Criar uma migration aditiva em `supabase/migrations` para `capture_sessions` e `captures`. A sessão guarda conta, hashes do código de pareamento e da credencial de envio, expirações, momentos de reivindicação, presença do computador e do celular, revogação e timestamps. A captura guarda conta, sessão de origem, URI privada no Supabase Storage, nome original, MIME validado, tamanho, criação, expiração e estado `uploading`, `available` ou `deleting`.
- Criar índices para a lista por conta e para a limpeza por expiração. Habilitar RLS nas tabelas e revogar acesso direto de `anon` e `authenticated`; todas as operações passam pela API Go. Não incluir tokens ou URI do Storage nas respostas de metadados.
- Implementar domínio e contratos próprios em `internal/features/capture`; manter queries e adaptador PostgreSQL dessa feature separados dos documentos de exame. Os serviços operacionais começam na etapa 1.2, quando passam a existir os casos de uso de pareamento. Atualizar os schemas e queries de origem do sqlc e gerar o código pelo comando do projeto.

**Aceite:** migration aplicada em PostgreSQL de teste; isolamento entre contas, índices, restrições e ausência de acesso direto pelas roles públicas verificados por teste de integração.

### 1.2 — Sessão do computador e código de pareamento

**Status: concluída em código e validada com testes unitários, de contrato HTTP e de integração em PostgreSQL local.**

- Adicionar rotas Huma protegidas pelo Supabase Bearer e pelo onboarding concluído: `POST /capture-sessions` cria uma sessão e devolve `session_id`, código para o QR e sua expiração; `GET /capture-sessions/current` devolve o estado atual; `POST /capture-sessions/{sessionId}/heartbeat` atualiza a presença; `DELETE /capture-sessions/{sessionId}` revoga a sessão.
- Gerar código aleatório de alta entropia, armazenar somente seu hash e aceitá-lo uma vez por até **5 minutos**. Criar nova sessão revoga a anterior da mesma conta de forma transacional. O código aparece somente na resposta de criação, com `Cache-Control: no-store`, e não entra em logs.
- O frontend enviará heartbeat a cada **20 segundos** enquanto a área autenticada estiver aberta. A API considera o computador presente apenas se o último heartbeat tiver no máximo **60 segundos**. Revogação impede novos envios imediatamente; ausência de presença os bloqueia até que ela seja restabelecida.

**Aceite:** conta A não consulta nem revoga sessão da conta B; QR expirado, reutilizado ou substituído não pode ser reivindicado; fechamento do computador interrompe uploads em até 60 segundos.

### 1.3 — Reivindicação e envio restrito do celular

**Status: concluída em código, com autenticação por `X-Capture-Token`, upload privado no Supabase Storage e testes unitários, HTTP e de integração PostgreSQL.**

- Adicionar `POST /capture-sessions/claim`, sem login Supabase, com `{ "code": "..." }`. A troca exige código válido, sessão não revogada e presença do computador; devolve `session_id`, uma credencial opaca de upload e `expires_at`, limitado a **12 horas após a reivindicação**. Armazenar somente o hash da credencial. Código inválido, expirado ou já usado recebe a mesma resposta de falha, sem revelar dados da conta. Aplicar `Cache-Control: no-store` também à resposta com a credencial.
- Adicionar `POST /capture-sessions/{sessionId}/mobile-heartbeat` e `POST /captures` com autenticação própria pela credencial de upload; ela não concede acesso às rotas protegidas pelo Supabase nem permite listar, ler ou excluir capturas. Cada heartbeat atualiza a presença do celular; a sessão é apresentada como conectada quando as presenças do celular e do computador tiverem no máximo **60 segundos**.
- O upload exige credencial válida, sessão não revogada e presença recente do computador. Validar exatamente um arquivo não vazio, limite de 5 MiB e conteúdo real de PDF/JPEG/PNG, sem confiar apenas no nome ou MIME enviado. Responder `201` com ID e metadados da captura; nunca aceitar paciente ou categoria no pedido.
- Reservar no banco uma captura em `uploading` antes de gravar o objeto privado de nome opaco no Supabase Storage; marcá-la `available` somente após concluir o upload. Se o upload ou a finalização falhar, marcar `deleting`, tentar remover o objeto e deixar a limpeza recuperar interrupções. Não expor o código do QR, a credencial ou o conteýo do documento nos logs.

**Aceite:** um celular sem login consegue enviar após reivindicar o QR; sem credencial, após revogação, após 12 horas ou sem presença do computador, o envio falha; cabeçalhos e extensões falsos não passam pela validação.

### 1.4 — Caixa de entrada e exclusão

**Status: concluída em código, com paginação, URL assinada limitada pela expiração, exclusão recuperável e testes unitários, HTTP e de integração PostgreSQL.**

- Adicionar rotas autenticadas `GET /captures` (lista paginada, mais recentes primeiro), `GET /captures/{captureId}/file` (URL assinada por até 5 minutos, limitada pela expiração da captura) e `DELETE /captures/{captureId}`. Consultas e exclusão exigem a conta proprietária; somente capturas `available` e não expiradas podem ser listadas ou receber URL.
- A lista retorna metadados e `expires_at`, independentemente do estado de conexão do celular. A credencial do celular não acessa essas rotas. A exclusão marca o registro, remove o objeto do Supabase Storage e conclui a remoção no banco; falha intermediária permanece recuperável por nova tentativa ou pela rotina de limpeza.

**Aceite:** capturas de outras contas não aparecem nem podem ser acessadas por ID; uma captura continua listada após o celular desconectar; exclusão repetida não deixa objeto acessível.

### 1.5 — Expiração, publicação e operação

**Status: implementação inicial concluída; correções 1.5.1–1.5.4 pendentes.**

- Criar um comando de limpeza executável como job agendado de hora em hora. Ele percorre, em lotes, capturas expiradas após **24 horas**, em `deleting` ou presas em `uploading` por mais de **1 hora**, remove objetos do Supabase Storage e depois os registros; somente então remove sessões vencidas sem capturas associadas. Repetir o job após falha deve ser seguro. Todas as consultas e operações recusam capturas expiradas mesmo antes da limpeza física.
- Registrar as rotas no OpenAPI gerado pelo Huma, com esquemas de segurança distintos para Supabase Bearer e credencial de captura; a rota de reivindicação é pública. Manter `AppError`, Problem Details e logs centralizados, sem dados clínicos ou segredos. Conectar handlers, repositórios, Supabase Storage e configuração no bootstrap da API.
- Aplicar a migration antes de publicar os endpoints. O cron horário fica fora do repositório: o ambiente executa `cmd/cleanup-captures`. Não criar `pg_cron` nem Vault.

**Aceite:** o job elimina arquivos e registros vencidos, recupera uploads e exclusões interrompidos e não afeta capturas válidas; o OpenAPI expressa corretamente as duas formas de autenticação; o fluxo de pareamento, upload, listagem, revogação e limpeza funciona com dois clientes HTTP independentes. Esse fluxo com dois clientes permanece verificação manual e não faz parte das correções abaixo.

#### 1.5.1 — Parar o lote sem progresso

**Status: pendente.**

`Cleanup` em `internal/features/capture/service.go` só interrompe a leitura de capturas quando o lote volta menor que o tamanho pedido. Se a exclusão no Storage falha, o registro continua candidato (`deleting` ou expirado). Com um lote cheio de falhas, a mesma consulta se repete até o timeout, e a limpeza de sessões não chega a rodar.

- Guardar os IDs já tentados nesta execução. Se um lote inteiro já foi visto, ou se nenhuma captura foi excluída, encerrar o laço de capturas e seguir para as sessões.
- Registrar também a falha de `MarkCaptureDeleting`, em vez de ignorar o erro. O registro permanece para a próxima execução.
- Ajustar o repositório falso de `internal/features/capture/service_test.go` para devolver de novo quem não foi excluído, como a SQL faz.

**Aceite:** um lote cheio em que todo `Delete` do Storage falha termina, não reprocessa o mesmo ID e ainda remove as sessões vencidas sem capturas.

#### 1.5.2 — Falha parcial encerra o comando com erro

**Status: pendente.**

O serviço devolve o relatório sem erro quando há falhas por captura. `POST /internal/jobs/cleanup-captures` já responde 500 nesse caso. O CLI em `cmd/cleanup-captures` só olha o erro fatal, registra aviso e termina com código 0.

- Sair com código 1 quando a execução falhar ou o relatório tiver erros.
- Não registrar a execução como concluída com sucesso nesse caso.
- Extrair a decisão do código de saída para uma função testável no pacote `main`.

**Aceite:** falha parcial e falha fatal encerram o comando com código 1; execução sem erros continua com código 0.

#### 1.5.3 — Testar a SQL e o token do job

**Status: pendente.**

A escolha do que apagar só existe na query e não passa pelo PostgreSQL. O handler do token também não tem teste.

- Em `internal/features/capture/postgres/repository_integration_test.go`, no mesmo schema isolado de `CAPTURES_TEST_DATABASE_URL`:
  - entra na limpeza: expirada, `deleting` ainda válida, `uploading` com `updated_at` anterior ao corte de 1 hora;
  - fica de fora: `available` não expirada e `uploading` recente;
  - `DeleteExpiredSessions` remove sessão revogada ou vencida sem capturas e preserva sessão com captura ou ainda válida.
- Testar o handler em `internal/features/capture/http/cleanup.go`: token ausente ou diferente responde não autorizado; relatório com erros responde falha interna; relatório limpo devolve as contagens.

**Aceite:** a SQL do job e a autenticação do endpoint interno ficam cobertas por teste; capturas válidas não entram na limpeza.

#### 1.5.4 — Comentário do token

**Status: pendente.**

- Em `.env.example`, remover a frase que manda gravar `CAPTURE_CLEANUP_TOKEN` no Vault. O token continua obrigatório porque protege o endpoint interno e o `config.Load` compartilhado pelo CLI.
- Ao concluir 1.5.1–1.5.3, atualizar o status destas subetapas.

**Aceite:** o exemplo de ambiente não promete Vault nem `pg_cron`; o cron horário permanece no ambiente, chamando o CLI.

## Verificação por entrega

Em cada subetapa, executar os testes Go pertinentes e `go test ./...`; nas subetapas de persistência, aplicar a migration e testar contra PostgreSQL isolado; após mudanças no schema, executar `go tool sqlc compile -f internal/infrastructure/database/postgres/sqlc/sqlc.yaml`. Conferir o OpenAPI exportado e `git diff --check`. Testar concorrência na reivindicação do QR e na exclusão, falha de Storage/banco, troca de conta, expiração com relógio controlado e ausência de segredos nos logs e respostas.
