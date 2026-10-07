<!-- docs/architecture/app-source-of-truth.md -->
# App-source-of-truth

Este documento formaliza a decisão de adotar o modelo "App-source-of-truth" para identidade e timestamps das entidades. Ou seja, a aplicação (domínio/serviços) é a fonte de verdade para `ID`, `created_at`, `updated_at` (e demais campos que não devem ser mutados automaticamente pelo banco), enquanto o banco persiste e valida via constraints.

## Objetivo
- Garantir consistência temporal e de identidade definida no domínio.
- Evitar divergências entre valores definidos pela aplicação e defaults/triggers do banco.
- Reduzir acoplamento a comportamentos "mágicos" do banco (ex.: `now()`/triggers), centralizando regras no código.

## Princípios
- IDs são gerados no domínio (UUID v7).
- `created_at` e `updated_at` são definidos no domínio (UTC), não por `now()` no SQL ou triggers.
- Soft delete (`deleted_at`) é definido pela aplicação quando necessário.
- O banco continua responsável por integridade: NOT NULL, tipos, UNIQUE, FK, CHECK.
- Regras de negócio (normalização, validação, idempotência) residem no domínio/serviços.

## Diretrizes de Código
- Domínio:
  - Construtores (`NewX`) devem gerar `ID` (`uuid.NewV7()`), setar `created_at` e `updated_at` como `time.Now().UTC()`.
  - Métodos de atualização devem alterar `updated_at` em UTC quando houver mudança.
- Repositórios:
  - Não devem gerar `ID` nem timestamps; devem aceitar os valores vindos da aplicação.
  - Não devem sobrescrever valores da entidade com defaults do banco.
  - Devem mapear erros de infra (unicidade, FK, etc.) para erros de repositório estáveis e o serviço mapeia para `AppError`.

## Diretrizes de SQL (sqlc)
- Inserts: incluir `created_at` e `updated_at` como parâmetros, evitando `now()`.
- Updates: receber `updated_at` como parâmetro; evitar `updated_at = now()`.
- Soft delete: idealmente receber `deleted_at` como parâmetro; quando for aceitável, `now()` pode ser mantido, desde que alinhado com a semântica de negócio.
- Remover triggers que alterem automaticamente `updated_at` ou `created_at`.
- Defaults em colunas podem existir como fallback, mas a aplicação sempre envia valores explícitos.

## Ajustes de contas
Arquivo: `internal/infrastructure/database/postgres/sqlc/sql/queries/account_queries.sql`
- `CreateAccountWithIdentity` envia IDs e timestamps definidos pelo domínio para conta e identidade.
- `UpdateAccountProfile` recebe `updated_at` da aplicação.
- `SoftDeleteAccount` ainda usa `now()` para `deleted_at` e `updated_at`; uma mudança para timestamps app-driven deve ser feita de forma explícita e coberta por testes.

## Ajustes planejados (Patients)
Arquivo: `internal/infrastructure/persistence/sqlc/sql/queries/patient_queries.sql`
- `CreatePatient`: remover `now(), now()` e incluir `created_at`, `updated_at` como parâmetros do INSERT.
- `UpdatePatient`: trocar `updated_at = now()` por `updated_at = $N`.
- `SoftDeletePatient`/`RestorePatient`: opcionalmente parametrizar `deleted_at` e `updated_at`.

## Alinhamento dos Repositórios
- `account/postgres`: confiar nos IDs e timestamps produzidos pelo domínio; manter o mapeamento de erros de unicidade e persistência sem vincular identidades por email ou CPF.
- `patient_repo`: já alinhado para preencher a entidade com o row; manter mapeamento de erros com `mapRepositoryError`.

## Migração de Banco (exemplo SQL)
Exemplo para `accounts`:
- Remover defaults/triggers automáticos e aceitar valores da aplicação.

```sql
-- Exemplos; ajuste nomes conforme seu schema
ALTER TABLE accounts ALTER COLUMN id DROP DEFAULT;
ALTER TABLE accounts ALTER COLUMN created_at DROP DEFAULT;
ALTER TABLE accounts ALTER COLUMN updated_at DROP DEFAULT;

-- Se houver trigger de updated_at, remover:
DROP TRIGGER IF EXISTS set_updated_at ON accounts;
DROP FUNCTION IF EXISTS trigger_set_updated_at();
```

Exemplo para `patients`:
```sql
ALTER TABLE patients ALTER COLUMN created_at DROP DEFAULT;
ALTER TABLE patients ALTER COLUMN updated_at DROP DEFAULT;
DROP TRIGGER IF EXISTS set_updated_at ON patients;
DROP FUNCTION IF EXISTS trigger_set_updated_at();
```

Observação: defaults podem ser mantidos como fallback, mas a aplicação deve sempre enviar valores explicitamente. Em ambientes legados, considere migração gradual.

## Testes & Verificações
- Criar entidade no domínio e persistir; verificar que `created_at/updated_at` no DB batem exatamente com os valores enviados.
- Atualizar entidade via domínio; conferir `updated_at` propagado pelo UPDATE.
- Soft delete: conferir semântica desejada (app envia `deleted_at` ou DB aplica `now()`).
- Verificar que nenhuma trigger altera timestamps implicitamente.

## Decisões em aberto
- `deleted_at`: manter `now()` no DB ou app-driven? Recomendação: app-driven para consistência.
- Defaults: manter como fallback de segurança ou remover completamente? Recomendação: manter apenas onde fizer sentido operacional.
