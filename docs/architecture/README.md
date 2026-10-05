<!-- docs/architecture/README.md -->
# Architecture

Descrição da arquitetura da Sonnda API, sua migração gradual por contexto, fluxos principais e decisões relevantes.

Este documento descreve **como a arquitetura está organizada**.  
As decisões não óbvias (o *porquê*) são registradas separadamente em ADRs.

## Documentação por contexto

- [Account](account/professional-activation.md): habilitação e mudança do tipo da conta.
- [Autorização](authz/README.md): política por ação e contrato `ProblemAuthorizer`.
- [Acesso a pacientes](access-control.md): vínculo e verificação de acesso ao prontuário.
- [Plano de problemas](../../problem-implementation-plan.md): sequência de implementação
  da autorização e da feature `patient/problem`.

---

## Visão geral

O backend está em camadas globais para contextos em `internal/features`, mantendo a separação entre domínio, aplicação, HTTP e persistência.

- **Application (`internal/application`)**  
  Orquestração e cross-cutting concerns.  
  - Use cases em `internal/application/usecase`; services em `internal/application/services` para os contextos ainda não migrados.
  - Bootstrapping (injeção de dependências) em `internal/application/bootstrap`.

- **API (`internal/api`)**  
  Implementações compartilhadas de adapters HTTP (inbound).  
  - Rotas em `internal/api/routes`; middlewares transversais em `internal/api/middleware`; presenter em `internal/api/presenter`.

- **Features (`internal/features`)**  
  Fluxos orientados a contexto de negócio.  
  - `auth` valida identidades externas e expõe `RequireBearer`.
  - `account` reúne serviços de perfil, onboarding, DTOs e mapeamento de erros.
  - `account/domain` contém `User`, `AccountType` e suas regras de validação, no pacote `accountdomain`.
  - `account/http` contém o handler de perfil e o middleware que resolve o usuário local e expõe `RequireRegisteredUser`.
  - `account/repository.go` define a interface de persistência; `account/postgres` implementa esse contrato usando o SQLC existente.
  - `patient/access` contém o checker, a listagem de pacientes acessíveis e os contratos de vínculo; seu handler HTTP atende `/v1/me/patients`.
  - `documentprocessing` reúne o processamento e gestão documental: consultas de documentos (`queries.go`), coordenação de rascunhos (`drafts.go`) e snapshot persistido (`snapshot.go`). Subpacotes `textextraction` e `labextraction` definem contratos de leitura e extração estruturada, e `extraction` coordena normalização e resumo.

- **Infrastructure (`internal/infrastructure`)**  
  Implementações concretas de persistência e integrações externas.  
  - **Persistence (`internal/infrastructure/persistence`)**: repositórios (sqlc/pgx), cache.
  - **Auth (`internal/infrastructure/auth`)**: Supabase auth provider.
  - **Document AI (`internal/infrastructure/documentai`)**: integração independente com Google Cloud Document AI, sem consumidor no fluxo laboratorial atual.
  - **Gemini (`internal/infrastructure/gemini`)**: cliente e extrator semântico estruturado baseado na API Google Gemini.

- **Kernel (`internal/kernel`)**  
  Preocupações transversais (cross-cutting concerns).  
  - **Error contract (`internal/kernel/apperr`)**: `AppError` e catálogo centralizado de códigos.
  - **Persistence (`internal/kernel/persistence`)**: sentinelas compartilhados de falha de persistência.
  - **Observability (`internal/kernel/observability`)**: logging (slog) com escopo de requisição.

Essas camadas representam **limites conceituais**, não apenas organização de pastas.

---

## Migração de account — aplicação, persistência e entidades

```text
internal/features/account/
├── domain/
│   ├── user.go
│   ├── account_type.go
│   └── user_test.go
├── service.go
├── service_impl.go
├── dto.go
├── error_map.go
├── onboarding.go
├── onboarding_dto.go
├── repository.go
├── postgres/
│   ├── repository.go
│   └── repository_test.go
└── http/
    ├── handler.go
    ├── middleware.go
    └── middleware_test.go
```

Os erros de conflito e usuário ausente pertencem ao contrato de account. A falha
genérica de persistência pertence a `internal/kernel/persistence/errors.go`.
O mapeamento de erros da aplicação deixa de importar a implementação Postgres.

As interfaces e a listagem de acesso a pacientes pertencem a `patient/access`.
`account` não depende mais do repositório de acesso. A conexão compartilhada e o
código sqlc permanecem em infraestrutura. O onboarding não depende mais de um serviço ou perfil profissional
separado; registra os tipos de conta existentes pelo serviço de account. O
cadastro HTTP continua criando `basic_care`, conforme o contrato atual.
Não há migração de queries nesta etapa.

Não houve mudança de banco ou regras de concessão. O contrato da listagem deixa
de expor `relation_type`. Os testes
em `internal/api/account_routes_test.go` verificam os fluxos pelas rotas reais,
com serviços de account e repositórios em memória.
Os testes do adaptador verificam parâmetros, conversões e erros com uma
implementação em memória da interface de queries do SQLC, sem acessar banco real.

---

## Processamento e extração de documentos (`documentprocessing`)

```text
internal/features/documentprocessing/
├── queries.go              # Consultas de documentos e textos extraídos (Service)
├── drafts.go               # Coordenação de criação, conferência e exclusão de rascunhos
├── snapshot.go             # Codificação e decodificação do snapshot persistido
├── dto.go                  # DTOs públicos de documentos
├── repository.go           # Contratos de persistência de documentos
├── textextraction/         # Contrato de leitura, qualidade e normalização de texto
├── labextraction/          # Contrato, tipos e schema da extração estruturada
├── extraction/             # Coordenação, normalização dos dados, avaliação e resumo
├── http/                   # Handlers Huma (extração temporária e rascunhos)
└── postgres/               # Adaptadores PostgreSQL para documentos e rascunhos
```

### Direção das dependências

1. **`textextraction` e `labextraction`**: Contratos e tipos folha, sem dependências de infraestrutura, HTTP ou da raiz de `documentprocessing`.
2. **`extraction`**: Coordena `textextraction` e `labextraction`. Não depende de HTTP, storage, banco de dados nem da raiz da feature.
3. **Raiz de `documentprocessing`**: `queries.go` para consultas públicas internas (`Service`), `drafts.go` para rascunhos e `snapshot.go` (`EncodeExtractionSnapshot` / `DecodeExtractionSnapshot`) para serialização de snapshots. Fica na raiz para evitar dependências circulares com `extraction`.
4. **Casos de uso**: `internal/application/usecase/labdocumentconfirmation` converte a fotografia conferida em histórico clínico (`patient/exam/laboratory`).

### Consumidores da extração

A extração de laudos atende a dois fluxos:
1. **Temporário (`StandaloneLabExtractionHandler` / `POST /lab-extractions`)**: Processamento em memória e arquivos temporários para conferência rápida, descartando o PDF em seguida. Não grava banco nem storage permanente.
2. **Rascunho (`Drafts` / `POST /patients/{patientId}/exam-documents`)**: Armazena o PDF no storage, grava a fotografia da extração e gera rascunho com status pendente para conferência do usuário.

*Nota:* A extração via terminal/CLI foi descontinuada e removida para evitar execuções sem autenticação e controle de acesso.

---

## Fluxo de request

1) **Middleware** autentica o usuário e adiciona informações ao contexto  
   (request_id, usuário autenticado, etc.).

2) **Handler HTTP**  
   - valida payload  
   - faz parsing de parâmetros  
   - monta o input do service  

3) **Service / Use case (camada App)**  
  - executa regras de negócio  
  - aplica políticas de acesso  
  - coordena chamadas a repositórios e serviços externos  

4) **Repository (Outbound)**  
  - executa queries via sqlc/pgx  
  - persiste ou consulta dados  

5) **Resposta HTTP**  
   - erros são normalizados para um contrato estável via `internal/kernel/apperr`
      - Veja `docs/architecture/error-handling.md`

---

## Persistência

- SQL definido em `internal/infrastructure/persistence/postgres/sqlc/sql`.
- `sqlc` gera código em `internal/infrastructure/persistence/postgres/sqlc/generated`.
- Repositórios de account ficam em `internal/features/account/postgres`; os demais continuam em `internal/infrastructure/persistence/postgres/repo`.
- Banco principal: PostgreSQL (Supabase).
- Soft delete usa `deleted_at`; consultas filtram `deleted_at IS NULL`.

---

## Observabilidade

- Logger baseado em `log/slog` (`internal/kernel/observability`).
- Variáveis:
  - `LOG_LEVEL`
  - `LOG_FORMAT`
- Um logger por request é injetado via middleware HTTP.

---

## Configuração

- Variáveis de ambiente definidas no ambiente (veja `.env.example` para referência).
- `APP_ENV` define o ambiente (`dev | prod`).
- Configurações carregadas na inicialização da aplicação (`internal/config`).

## OpenAPI

- As rotas Huma são a fonte de verdade do contrato HTTP.
- O CI exporta `artifacts/openapi.json` e o publica como artefato imutável
  identificado pelo SHA do commit da API.
- `/openapi.json`, `/openapi.yaml` e `/docs` continuam servidos dinamicamente
  pelo Huma.

---

## Bootstrap e rotas

- Bootstrap faz o wiring (repos, services e handlers) em `internal/application/bootstrap`.
- `AccountModule` reúne handler de perfil/onboarding e middleware de usuário registrado.
- As rotas HTTP vivem em `internal/api/routes.go` (API REST).  
- Níveis de acesso:
  - público
  - autenticado
  - registrado

---

## Decisões arquiteturais (ADR)

Algumas decisões importantes do projeto **não são óbvias apenas pela leitura do código**.  
Para preservar o contexto dessas escolhas ao longo do tempo, o Sonnda adota o uso de **Architecture Decision Records (ADR)**.

Os ADRs documentam:
- o contexto da decisão
- a decisão tomada
- alternativas consideradas
- consequências

Os ADRs vivem em:
`docs/architecture/adr/`.

---

## Controle de acesso aos pacientes

O pacote `internal/features/patient/access` centraliza a checagem de acesso por
vínculo. `RequireAccess` permite acesso ao dono do paciente ou
a um usuário com vínculo ativo; os demais recebem 403. Pacientes, exames e
laudos compartilham essa regra.

A mesma feature atende `GET /v1/me/patients`. A rota permanece estável, enquanto
o serviço, o handler e os DTOs deixam de pertencer a `account`.

O checker não decide permissões por ação. Para problemas do paciente, `authz`
combina o acesso confirmado pelo checker com o `AccountType` persistido. O tipo
da conta continua sem conceder acesso automático a pacientes.

Detalhes: `docs/architecture/access-control.md`.
