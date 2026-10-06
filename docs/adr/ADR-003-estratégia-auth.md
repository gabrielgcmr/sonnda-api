<!-- docs/architecture/adr/ADR-003-estratégia-auth.md -->
# ADR-003 — Estratégia de Autenticação: Supabase Auth com Bearer Token (API)

Status: Aceito
Data: 2026-01-29
Decisores: Equipe Sonnda
Contexto: Autenticação e Autorização (API + Mobile/SPA futura)

## Contexto

A plataforma Sonnda expõe uma API HTTP (JSON) consumida por aplicações externas e mobile. A camada Web SSR foi removida e a interface web será uma SPA separada no futuro.

A autenticação precisa ser simples, stateless e compatível com clients modernos (mobile/SPA), sem necessidade de sessão server-side.

## Decisão

Foi decidido adotar **Supabase Auth** como Identity Provider (IdP), utilizando **Bearer Token (JWT)** para todas as rotas autenticadas da API.

- O cliente autentica no Supabase e envia `Authorization: Bearer <access_token>`.
- O backend valida o JWT via OIDC/JWKS.
- O middleware de autenticação injeta a identidade no contexto da requisição.

## Consequências

### Positivas

- Fluxo único e simples (Bearer Token) para API, mobile e SPA.
- Sem sessão server-side e sem cookies HttpOnly.
- Compatível com integração padrão de Supabase.

### Negativas / Trade-offs

- Requer controle adequado de storage de token no client (SPA/mobile).
- Revogação imediata depende das políticas do provedor (curto TTL + refresh).

## Alternativas Consideradas

- **Auth0 + sessão web**: desnecessário sem SSR.
- **Firebase Auth**: substituído por Supabase por simplicidade e alinhamento com o stack.

## Decisão Final

Supabase Auth com Bearer Token é a estratégia oficial para autenticação da API.
