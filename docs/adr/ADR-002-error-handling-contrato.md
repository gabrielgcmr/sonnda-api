<!-- docs/architecture/adr/ADR-002-error-handling-contrato.md -->
# ADR-002 — Contrato de erros com AppError + Problem Details (RFC 9457)

**Status:** Aceito  
**Data:** 2026-01  
**Contexto:** Sonnda API — padronização do fluxo de erros do domínio até HTTP (Gin)

---

## Contexto

A API precisa expor um **contrato estável** de erros para clientes (frontend/mobile), sem acoplar o domínio a HTTP e sem vazar detalhes internos.

Problemas observados antes desta decisão:

- Inconsistência entre handlers e módulos ao responder erros (cada um “mapeava” do seu jeito).
- Dificuldade em manter observabilidade: erros internos eram perdidos (sem `%w`) ou vazavam para o cliente via `err.Error()`.
- Risco de dependência circular ao tentar centralizar “mapeamento HTTP” no mesmo pacote onde vivem os códigos de erro.
- Ruído de logs: múltiplos logs por request com baixo valor, dificultando operação.

Além disso, existe infraestrutura de observabilidade já consolidada:

- Logger `log/slog` **por request**, injetado no `context.Context` pelo middleware de AccessLog.
- Middleware de Recovery que captura `panic`, gera stacktrace e responde 500.

---

## Decisão

Padronizar o tratamento de erros em três peças (por camada):

1) **Contrato de erro da aplicação:** `internal/kernel/apperr`
   - `AppError{ Code, Message, Cause }` implementa `error` e suporta `Unwrap()` (para `errors.Is/As`).
   - Catálogo de códigos em `internal/kernel/apperr/catalog.go` (`ErrorCode`).
   - Política de log por erro em `internal/kernel/apperr/logging.go` (`ErrorCodeOf`, `LogLevelOf`).

2) **Presenter HTTP (adaptador Gin):** `internal/api/presenter`
   - `ToProblem(err) -> (status, payload)` produz **RFC 9457** em `application/problem+json`.
   - `ErrorResponder(c *gin.Context, err error)` escreve a resposta e adiciona metadados no `gin.Context`:
     - `error_code`
     - `http_status`
     - `error_log_level`
   - `ErrorResponder` evita log detalhado em 4xx (deixa o AccessLog cobrir); em 5xx, registra log detalhado com a cadeia `%w`.

3) **AccessLog como log principal por request:** middleware HTTP
   - Sempre 1 log por request (status/latência/route/etc.).
   - Enriquecido com `error_code` e `error_log_level` quando presentes.
   - Mantém a linha de log mesmo em 5xx (métrica operacional de falha).

O domínio (`internal/domain/...`) continua **agnóstico de HTTP**: valida invariantes e retorna `error` (sentinelas/tipos), podendo fazer wrapping com `%w` para preservar causa semântica.

Detalhes do fluxo e do contrato: `docs/architecture/error-handling.md`.

---

## Alternativas consideradas

1) **Mapear erros no handler (por endpoint)**
   - Rejeitado: gera inconsistência e duplicação; cada novo erro exige “caça aos handlers”.

2) **Expor `err.Error()` para o cliente**
   - Rejeitado: vaza detalhes internos e quebra contrato quando mensagens mudam.

3) **Usar strings como contrato (“cpf_already_exists”) espalhadas**
   - Rejeitado: difícil de versionar/organizar e tende a proliferar sem catálogo.

4) **Misturar catálogo de códigos com mapeamento HTTP no mesmo pacote**
   - Rejeitado: aumenta risco de dependência circular (app <-> http).

---

## Consequências

**Positivas**
- Contrato estável (`code`/`message`) e independente de implementação interna.
- Preserva causa técnica com `%w` para observabilidade, sem vazamento no payload.
- Centraliza mapeamento code -> status e a política de logging.
- Reduz ruído: 4xx fica só no AccessLog; 5xx ganha log detalhado adicional.

**Negativas / trade-offs**
- Services precisam “traduzir” erros esperados para `*apperr.AppError` (parte do design).
- O catálogo de codes precisa ser mantido com disciplina (checklist no documento).
