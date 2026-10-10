<!-- docs/architecture/error-handling.md -->
# Error Handling (Go + Huma/Gin)

## Rotas Huma e migração do contrato HTTP

As operações registradas por Huma (`huma.Register`) usam o modelo de erro
RFC 9457 padrão da Huma. Elas escrevem erros com `huma.WriteErr`,
`huma.NewError` ou `huma.Error*`.

`AppError` continua sendo a classificação interna de falhas entre domínio,
aplicação, autenticação e persistência. Na fronteira Huma, ele fornece o
status HTTP e uma mensagem segura. As extensões públicas do antigo formato
Sonnda (`code`, `traceId`, `violations`, `instance` e `timestamp`) não são
devolvidas pelo modelo padrão da Huma.

```text
domínio/aplicação -> AppError (classificação interna)
                   -> StatusError / WriteErr da Huma
                   -> application/problem+json da Huma
```

| Tipo de rota          | Corpo de erro HTTP                   | Serialização               |
| --------------------- | ------------------------------------ | -------------------------- |
| Huma (`/me`)          | RFC 9457 padrão da Huma              | Huma                       |
| Gin ainda não migrada | Problem Details Sonnda com extensões | `presenter.ErrorResponder` |

A coexistência é temporária. Não adicionar novas rotas ao formato
Gin/Presenter. Quando todas as rotas forem Huma, o presenter HTTP legado e
seus campos públicos devem ser removidos; `AppError` permanece como
classificação interna e para a política de logs.

As rotas Huma não devem chamar `presenter.ErrorResponder` nem desligar os
transformers da Huma para imitar o formato anterior. Os schemas, links e a
documentação produzidos pela configuração padrão fazem parte do comportamento
adotado.

Este documento descreve a arquitetura de tratamento de erros da Sonnda API, inspirada em Hexagonal/Clean Architecture e aplicada de forma pragmática em Go.

### Referências

- ADR: `docs/architecture/adr/ADR-002-error-handling-contrato.md`
- Catálogo de códigos: `internal/kernel/apperr/catalog.go`
- Política de log por erro: `internal/kernel/apperr/logging.go`
- Presenter HTTP legado (Gin): `internal/api/presenter` (veja `ErrorResponder`)
- Middleware de AccessLog: `internal/api/middleware/access_log.go`
- Middleware de Recovery: `internal/api/middleware/recovery.go`

---

## Objetivos

- Definir **um contrato estável** de erro para clientes (frontend/mobile).
- Manter o **domínio agnóstico de HTTP** (sem status codes, sem Gin).
- Preservar causas técnicas com `%w` (observabilidade), mas **não vazar detalhes** na resposta HTTP.
- Evitar dependência circular entre pacotes.
- Evitar logs duplicados com baixo valor (1 log por request + 1 log detalhado só em 5xx).

---

## Contrato HTTP de erro legado (Gin)

Formato padrão (RFC 9457 - Problem Details):

```json
{
  "type": "urn:sonnda:problem:validation_failed",
  "title": "Falha de validação",
  "status": 400,
  "detail": "payload inválido",
  "instance": "urn:sonnda:request-id:8a0f8a9b-2e1c-4c46-a2b1-1a6f8a6c2e44",
  "code": "VALIDATION_FAILED",
  "violations": [{ "field": "email", "reason": "required" }],
  "traceId": "8a0f8a9b-2e1c-4c46-a2b1-1a6f8a6c2e44",
  "timestamp": "2026-02-06T12:34:56Z"
}
```

- Content-Type: `application/problem+json`
- `code`: identificador estável (contrato com clientes).
- `detail`: mensagem segura (não expõe detalhes internos).

Implementado por `internal/api/presenter.ToProblem` + `internal/api/presenter.ErrorResponder`.

---

## Fluxo legado por camada (Gin)

```
HTTP Handler (Gin)
  ├─ valida/bind/parse (erros de fronteira)
  │    └─ cria *apperr.AppError { Code, Message, Cause }
  ├─ chama Service (internal/application/...)
  │    └─ Service converte erros esperados em *apperr.AppError
  └─ escreve resposta (internal/api/presenter)
       ├─ ToProblem(err) => (status, problem_details)
       └─ ErrorResponder(c, err) => resposta + context keys + logging (5xx)
```

---

## Camadas e responsabilidades

### Domain (`internal/domain/...`)

- Retorna `error` (sentinelas/tipos) e valida invariantes.
- Pode fazer wrapping com `%w` para preservar a causa semântica sem “quebrar” `errors.Is`.
- Nunca importa HTTP e não conhece status codes.

### Application (`internal/application/...`)

Responsável por transformar erros “relevantes” em um erro de contrato: `*apperr.AppError`.

`*apperr.AppError` carrega:
- `Code` (`apperr.ErrorCode`) — contrato estável
- `Message` — seguro para o cliente
- `Cause` — erro interno (opcional), preservado via `Unwrap()` (suporta `errors.Is/As`)

Definição: `internal/kernel/apperr/error.go`

O catálogo de códigos fica em `internal/kernel/apperr/catalog.go`.

### HTTP adapter (presenter canonical)

O presenter canonical é o package `internal/api/presenter` que exporta:
- `ToProblem(err) (status int, body problem.Details)`
- `ErrorResponder(c *gin.Context, err error)` — presenter para endpoints da API

---

## Mapeamento `code -> status`

O mapeamento é centralizado em `internal/adapters/inbound/http/shared/httperr/http_mapper.go` e segue as regras do catálogo `internal/kernel/apperr`.

---

## Padrão de uso nos handlers

Regras:
- Handler deve chamar `internal/api/presenter.ErrorResponder(c, err)`.
- Erros de fronteira (auth/bind/parse) são convertidos para `&apperr.AppError{Code, Message, Cause}` no handler e passados para o presenter.
- Erros do service/usecase **já devem** voltar como `*apperr.AppError` quando forem esperados.

Exemplo (padrão):

```go
import (
  "github.com/gabrielgcmr/sonnda/internal/api/presenter"
  "github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

if err := c.ShouldBindJSON(&req); err != nil {
  presenter.ErrorResponder(c, &apperr.AppError{
    Code: apperr.VALIDATION_FAILED,
    Message: "payload inválido",
    Cause: err,
  })
  return
}

out, err := h.svc.Register(c.Request.Context(), input)
if err != nil {
  httperr.APIErrorResponder(c, err)
  return
}
```

---

## Checklist (novo code / novo erro)

1) Defina o `ErrorCode` em `internal/kernel/apperr/catalog.go`.
2) Garanta o status em `internal/adapters/inbound/http/shared/httperr/http_mapper.go`.
3) Defina o nível de log em `internal/kernel/apperr/logging.go` (se necessário).
4) No service, mapeie o erro do domínio para `*apperr.AppError` (ex.: `mapXDomainError`).
5) No handler, use `httperr.APIErrorResponder` ou `httperr.WebErrorResponder` (sem conversão manual para status/JSON).
6) Adicione/ajuste testes no service e (se fizer sentido) no presenter HTTP.
7) Atualize este documento e o ADR.
