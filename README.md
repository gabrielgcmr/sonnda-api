<!-- README.md -->
# Sonnda

Server da plataforma Sonnda, voltada para atencao primaria a saude e para organizacao do historico clinico centrado no paciente.

A Sonnda resolve um problema recorrente na pratica clinica: pacientes precisam carregar pilhas de exames, perdem documentos e o cuidado fica fragmentado. A proposta e permitir que o paciente armazene e compartilhe seu historico (sem depender de papel/WhatsApp), e que profissionais de saude consigam visualizar e evoluir o paciente com base em um historico longitudinal acessivel via web.

## O que este repositorio entrega (MVP)

- cadastro e gerenciamento de pacientes;
- upload e processamento de exames laboratoriais;
- extracao automatica de dados estruturados via Google Cloud Document AI;
- armazenamento seguro em PostgreSQL (Supabase);

> Atencao: este repositorio nao deve conter dados reais de pacientes nem arquivos de configuracao sensiveis (`.env`).

---

## Sumario

- [Sonnda](#sonnda)
  - [O que este repositorio entrega (MVP)](#o-que-este-repositorio-entrega-mvp)
  - [Sumario](#sumario)
  - [OpenAPI](#openapi)
  - [Arquitetura](#arquitetura)
  - [Stack Tecnologico](#stack-tecnologico)
  - [Logging](#logging)
  - [Tratamento de Erros](#tratamento-de-erros)
    - [Regras centrais](#regras-centrais)
    - [AppError](#apperror)
  - [Configuracao de ambiente (dev/prod)](#configuracao-de-ambiente-devprod)
  - [Estrutura de Pastas](#estrutura-de-pastas)

---

## OpenAPI

O contrato HTTP é gerado dinamicamente pelo Huma a partir das rotas da API.
Use `make openapi-export` para criar `artifacts/openapi.json`. Em cada consumidor,
execute `task openapi` para regenerar o cliente a partir desse arquivo local.

- Erros HTTP: RFC 9457 (Problem Details) via `application/problem+json`.

## Arquitetura

A aplicação é organizada principalmente por contexto de negócio em `internal/features`. A migração é incremental; nem todas as features têm a mesma estrutura interna.

- **Features (`internal/features`)**: `account`, `auth`, `documentprocessing` e `patient` (`access`, `profile` e `exam`). Cada contexto mantém modelos, contratos, serviços, handlers HTTP e adapters de persistência conforme necessário.
- **Document processing**: coordena documentos e extrações; seus pacotes incluem `domain`, `extraction`, `labextraction`, `textextraction`, `http` e `postgres`. O contrato de armazenamento de arquivos pertence a essa feature.
- **Domain compartilhado (`internal/domain`)**: conceitos puros usados por mais de uma feature, atualmente incluindo demografia. Modelos específicos ficam dentro da feature proprietária.
- **Application (`internal/application`)**: `bootstrap` compõe módulos e dependências; `usecase` contém fluxos entre features, como criação de pacientes e confirmação de laudos.
- **API (`internal/api`)**: configura Gin e Huma, compõe rotas e mantém middleware, helpers e tradução de erros HTTP. Handlers de negócio ficam junto das features. As rotas Huma são a fonte do OpenAPI.
- **Infrastructure (`internal/infrastructure`)**: adapters concretos para PostgreSQL/sqlc, autenticação, Supabase Storage, Redis e integrações de extração com Gemini, Document AI e comandos locais.
- **Kernel (`internal/kernel`)**: contratos transversais de erro, persistência e observabilidade. `internal/config` concentra a configuração da aplicação e integrações.

---

## Stack Tecnologico

**Backend:** API RESTful em Go

- **Framework web:** Gin com Huma para rotas e OpenAPI
- **Banco de dados:** PostgreSQL (gerenciado via Supabase) + Redis (Upstash)
- **Geração de código SQL:** SQLC
- **Autenticação:** Supabase Auth (JWT)
- **Armazenamento de arquivos:** Supabase Storage, com buckets privados para documentos permanentes e capturas temporárias
- **Processamento de documentos:** extração textual por adapter de comandos e extração estruturada com Gemini; há também um adapter para Google Cloud Document AI
- **Containerização:** Docker / docker-compose

**Ferramentas de desenvolvimento:**

- Air (live reload)
- SQLC (geração de código SQL)
- Make (automação de tarefas)
- Docker + docker-compose (containerização)

---

## Logging

- O app usa `log/slog` via `internal/kernel/observability` (logger com escopo de requisição é injetado pelo middleware HTTP).
- Configure com `LOG_LEVEL` (`debug|info|warn|error`) e `LOG_FORMAT` (`text|json|pretty`).

## Tratamento de Erros

Este projeto usa um **contrato de erro centralizado** baseado em `AppError`.

### Regras centrais

- **NÃO retorne strings brutas como contratos de erro.**
- **NÃO exponha `err.Error()` em respostas HTTP.**
- **NÃO construa JSON de erro manualmente em handlers ou middlewares.**

### AppError

- Erros de nível de aplicação devem ser representados como `*apperr.AppError`.
- Localização: `internal/kernel/apperr`
- `AppError` contém:
  - `Kind` (`ErrorKind`) - contrato estável e legível por máquina
  - `Message` - mensagem segura e legível por humanos
  - `Cause` - erro interno opcional (wrapped com `%w`)
- **Prefira usar construtores helper** de `internal/kernel/apperr/factory.go` ao invés de construí-los manualmente.
- Serviços e casos de uso **devem retornar `AppError` para falhas conhecidas** (validação, conflitos, não encontrado, erros de infraestrutura).
- Domínio **nunca** importa HTTP, Gin ou `apperr`.
- Handlers Huma convertem erros de aplicação com `humaerror.From(err)`; middlewares usam `humaerror.Write(api, ctx, err)`.
- Use respostas Problem Details padrão do Huma. Não exponha causas internas nem construa JSON de erro manualmente.
- `internal/api/presenter.ErrorResponder` permanece apenas para consumidores Gin legados; não use em handlers Huma novos.

## Configuracao de ambiente (dev/prod)

- Para dev local, copie o exemplo: `cp .env.example .env`.
- O app **nao** carrega `.env` automaticamente. Exporte as variaveis no shell (ou use `direnv`).

---

Comandos:

```bash
make dev
```

## Estrutura de Pastas

Estrutura atual do projeto:

```text
.
├── cmd/
│   ├── api/                        # Inicialização da API
│   └── openapi-export/             # Exportação do contrato OpenAPI
├── docs/
│   └── architecture/               # Arquitetura e ADRs
├── internal/
│   ├── api/                        # Gin/Huma, rotas e middleware compartilhados
│   ├── application/                # Bootstrap e casos de uso
│   ├── config/                     # Configuração
│   ├── domain/                     # Conceitos compartilhados
│   ├── features/                   # Contextos de negócio e seus adapters
│   ├── infrastructure/             # PostgreSQL, Supabase Storage e integrações externas
│   └── kernel/                     # Erros, persistência e observabilidade
├── static/                         # Assets embutidos
├── supabase/                       # Configuração e migrations do banco
├── AGENTS.md                       # Instruções para agentes
├── Makefile                        # Comandos de desenvolvimento e geração
└── README.md                       # Documentação do projeto
```
