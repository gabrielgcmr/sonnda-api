<!-- docs/architecture/authz/README.md -->
# Autorização

Este documento descreve somente como a API decide se uma conta pode executar
uma ação sobre os problemas de um paciente.

## Fontes confiáveis

A decisão usa dados carregados pelo backend:

- `account_id` obtido da identidade autenticada;
- `account_type` persistido em `account`;
- acesso ativo ao paciente, validado por `patient/access`;
- `patient_id` da operação.

`relation_type`, `owner_user_id`, identidade própria e condição de cuidador não
concedem nem retiram permissões. Valores enviados no payload também não definem
o tipo da conta ou seu acesso.

Uma conta `professional` não recebe acesso automático a pacientes. Primeiro ela
precisa passar pela mesma verificação de acesso aplicada às demais contas.

## Política para problemas

| Ação | `professional` com acesso | `basic_care` com acesso |
| --- | --- | --- |
| Listar problemas | Sim | Sim |
| Consultar problema | Sim | Sim |
| Consultar histórico | Sim | Sim |
| Criar problema | Sim | Não |
| Editar nome ou CID | Sim | Não |
| Classificar cronicidade | Sim | Não |
| Resolver problema | Sim | Sim |
| Reabrir problema | Sim | Não |
| Retificar registro | Sim | Não |
| Unificar problemas | Sim | Não |

A autorização para resolver indica apenas que a conta pode solicitar a mudança.
`patient/problem` ainda precisa verificar se o problema foi explicitamente
classificado como agudo e se a transição de estado é válida.

## Interface

`patient/problem` consome somente `authz.ProblemAuthorizer`:

```go
Authorize(ctx context.Context, accountID, patientID uuid.UUID, action ProblemAction) error
```

O fluxo é:

1. A camada HTTP obtém `accountID` da autenticação e `patientID` da rota.
2. `PatientContextResolver` carrega a conta e exige acesso ao paciente.
3. `ProblemAuthorizer` aplica a política correspondente à ação.
4. `patient/problem` valida as regras clínicas e executa a operação.
5. O handler traduz falhas com `humaerror.From(err)`.

HTTP e `patient/problem` não devem repetir a matriz de permissões.

## Erros

- Identidade ausente: `AUTH_REQUIRED`.
- Conta inválida, falta de acesso, paciente divergente, ação desconhecida ou ação
  profissional solicitada por `basic_care`: `ACCESS_DENIED`.
- Falha de persistência: `INFRA_DATABASE_ERROR`.
- Dependência não configurada: `INTERNAL_ERROR`.

## Limites

`authz` decide quem pode solicitar a ação. As regras de cronicidade, transição,
retificação, unificação, concorrência e auditoria pertencem a `patient/problem`.
A habilitação que altera uma conta para `professional` pertence a `account`.

Plano de entrega: [Plano de implementação de problemas](../../../problem-implementation-plan.md).
