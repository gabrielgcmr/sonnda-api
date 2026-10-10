<!-- docs/architecture/account/identity-onboarding.md -->
# Identidade, conta e onboarding

O Supabase autentica a identidade externa. A API resolve essa identidade pelo
par validado `issuer + subject` e provisiona, quando necessário, uma conta
Sonnda `basic_care` com UUID próprio. Email é informativo e nunca é usado para
vincular contas.

## Contrato HTTP

- `GET /me` resolve ou provisiona a conta e retorna `Cache-Control: private, no-store`.
- `PATCH /me` atualiza somente `full_name`, `birth_date`, `cpf` e `phone`.
- `DELETE /me` desativa uma conta existente sem provisionar outra.
- Não existem `POST /me` nem `PUT /me`.

`GET /me` e `PATCH /me` retornam:

```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "email": "person@example.com",
  "account_type": "basic_care",
  "profile": {
    "full_name": null,
    "birth_date": null,
    "cpf": null,
    "phone": null
  },
  "onboarding_completed": false,
  "created_at": "2026-10-06T12:00:00Z",
  "updated_at": "2026-10-06T12:00:00Z"
}
```

No PATCH, campo omitido preserva o valor, `null` remove o valor e string vazia
remove CPF ou telefone. Nome e nascimento vazios são inválidos. UUID, email,
tipo da conta, identidade e `onboarding_completed` são imutáveis pelo endpoint.

O onboarding está concluído quando nome e nascimento são válidos. Enquanto
estiver pendente, `/me` permanece acessível, mas pacientes, documentos, exames
e ativação profissional retornam 403.

## Migração dos clientes

Web e mobile devem:

1. remover a criação por `POST /me`;
2. trocar atualizações de `PUT /me` para `PATCH /me`;
3. ler dados pessoais dentro de `profile`;
4. usar `onboarding_completed` para decidir a navegação;
5. tratar CPF e telefone como opcionais;
6. obter o OpenAPI imutável publicado pelo SHA da API antes de regenerar clientes.

As duas listagens de pacientes permanecem disponíveis em `GET /patients` e
`GET /me/patients`.

## Persistência e segurança

`accounts` contém o perfil e o estado da conta; `account_identities` mantém as
associações autenticadas. As tabelas têm RLS habilitado e não concedem acesso
direto a `anon` ou `authenticated`. A função usada pelas políticas de pacientes
fica no schema privado e só resolve contas ativas com onboarding concluído.

Para validar a migration localmente, configure `ACCOUNTS_TEST_DATABASE_URL` com
uma instância Supabase local isolada e execute:

```sh
go test ./internal/features/account/postgres -run MigrationIntegration -v
```
