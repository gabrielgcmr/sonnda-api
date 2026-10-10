# Separar identidade, conta e onboarding do Sonnda

## Resultado esperado

Supabase continua responsável pela autenticação. A API provisiona automaticamente uma conta com UUID próprio e permite preencher o perfil aos poucos, sem `POST /me`.

Decisões confirmadas:

- Implementação somente na API; ajustes de web/mobile ficam para uma entrega posterior.
- Nome e nascimento válidos concluem o onboarding.
- CPF e telefone são opcionais na conta. CPF de paciente continua obrigatório.
- Recursos de pacientes, documentos/exames e ativação profissional exigem onboarding concluído.
- Permanecem as duas listagens: `GET /patients` e `GET /me/patients`.
- `DELETE /me` desativa a conta e impede seu reprovisionamento automático.

## Etapas de implementação

### 1. Migrar o banco preservando contas e vínculos

Criar uma nova migration em `supabase/migrations`, sem alterar migrations já aplicadas.

- Renomear `users` para `accounts`, preservando UUIDs, dados e referências existentes. Colunas como `owner_user_id` podem manter seus nomes nesta entrega.
- Criar `account_identities` com `account_id`, `issuer`, `subject`, `email` opcional e timestamps. Usar `UNIQUE (issuer, subject)` e FK para `accounts`.
- Migrar cada identidade existente para essa tabela, incluindo o email como cópia informativa da identidade. Remover `auth_issuer`, `auth_subject` e `email` de `accounts` depois de verificar o backfill.
- Permitir `NULL` em `full_name`, `birth_date`, `cpf` e `phone`. Manter CPF único quando preenchido, inclusive em contas desativadas.
- Remover a unicidade por email: email não identifica nem vincula contas.
- Atualizar a função `current_app_user_id()` para consultar a nova associação usando **issuer e subject**, considerando somente contas ativas com onboarding completo.
- Habilitar RLS nas tabelas novas e permitir escrita em contas/identidades somente pelo backend. Remover a política antiga de CRUD direto do perfil e os grants de `anon`/`authenticated` nessas tabelas. Preservar a função necessária às políticas de pacientes, com acesso restrito e referências qualificadas. [Referência de segurança do Supabase](https://supabase.com/docs/guides/api/securing-your-api).
- Atualizar os schemas e queries de origem do sqlc e regenerar os pacotes dependentes.

**Conclusão da etapa:** contas antigas mantêm os mesmos IDs, pacientes, concessões de acesso e histórico.

### 2. Separar os modelos de conta e identidade

- Substituir o modelo de domínio `User` por `Account`, ajustando seus consumidores.
- A conta contém UUID, tipo de conta, perfil, timestamps e estado de desativação. A associação autenticada fica em um modelo separado, pertencente à feature `account`.
- Remover `PrincipalID()` dos modelos de conta e identidade; o domínio utiliza exclusivamente o UUID da conta.
- Representar campos não preenchidos com valores opcionais, sem datas zero ou strings vazias.
- Separar validação de perfil parcial da regra de conclusão do onboarding.
- Calcular `onboarding_completed` a partir de nome e nascimento válidos, sem armazenar um booleano independente.
- Preservar as regras atuais de formato: nome de 2–120 caracteres, nascimento válido e não futuro, CPF com 11 dígitos e telefone com 10–15 dígitos, podendo começar com `+`. A mudança não adiciona validação de dígitos verificadores do CPF.

**Conclusão da etapa:** uma conta mínima é válida, mesmo sem nenhum dado de perfil.

### 3. Implementar provisionamento automático e atualização atômica

Substituir o onboarding que cria usuários por operações de resolução/provisionamento e atualização do perfil.

- Resolver a conta pelo par `issuer + subject` do JWT validado. O `sub` utilizado é o usuário do Supabase, inclusive quando ele autentica pelo Google. [Referência de claims JWT](https://supabase.com/docs/guides/auth/jwts).
- Se a identidade não existir, criar conta `basic_care` e associação na mesma transação, com perfil inicialmente vazio.
- Serializar o provisionamento da mesma identidade com lock transacional no PostgreSQL; manter a restrição única como garantia adicional. Requisições concorrentes devem retornar o mesmo UUID, sem contas órfãs.
- Nunca vincular contas automaticamente por email ou CPF. A estrutura suporta múltiplas identidades, mas endpoints de vinculação ficam fora desta entrega.
- Procurar também contas desativadas: encontrá-las deve produzir 403, sem criar outra conta.
- Sincronizar o email informativo da identidade a partir do token autenticado; não aceitar alteração por `PATCH /me`. Email ausente não impede provisionamento.
- Atualizar o perfil em transação com lock da conta, evitando perda de alterações concorrentes em campos diferentes.
- Preservar idempotência: atualizações sem mudanças não alteram `updated_at`.

**Conclusão da etapa:** o frontend não precisa criar uma segunda conta após autenticar no Supabase.

### 4. Publicar o novo contrato HTTP

`GET /me` e `PATCH /me` retornam o mesmo formato:

```json
{
  "id": "UUID da conta Sonnda",
  "email": "gabriel@example.com",
  "account_type": "basic_care",
  "profile": {
    "full_name": null,
    "birth_date": null,
    "cpf": null,
    "phone": null
  },
  "onboarding_completed": false,
  "created_at": "...",
  "updated_at": "..."
}
```

- `GET /me`: resolve ou provisiona a conta e retorna 200. Usar `Cache-Control: private, no-store`.
- `PATCH /me`: aceita os campos de perfil na raiz, mantendo o exemplo original:

```json
{
  "full_name": "Gabriel Rebouças",
  "birth_date": "1993-01-01"
}
```

- Campo omitido preserva o valor; `null` remove o valor. CPF/telefone vazios viram `null`; nome/nascimento vazios são inválidos.
- Distinguir ausência, `null` e valor no DTO; ponteiros simples não bastam para esse contrato.
- Permitir limpar nome/nascimento com `null`, fazendo o onboarding voltar a pendente.
- Não aceitar alterações de UUID, identidade, email, tipo de conta ou `onboarding_completed`.
- Remover `POST /me` e `PUT /me`, sem aliases temporários.
- Separar os middlewares de autenticação, resolução da conta e exigência de onboarding. `/me` continua acessível com perfil incompleto; os recursos de negócio retornam 403 enquanto o onboarding estiver pendente.
- `DELETE /me`: fazer exclusão lógica, preservando identidades e vínculos. Retornar 204 também em repetição para conta já desativada; retornar 404 se nunca existiu conta. Essa rota não provisiona contas.
- Preservar ambas as listagens de pacientes e suas respostas atuais.
- Manter `AppError`, Problem Details e a arquitetura centralizada de logs; adicionar erros de onboarding pendente e conta desativada sem expor códigos internos no JSON público.

**Conclusão da etapa:** existe um único fluxo de perfil, baseado em leitura e atualização.

### 5. Verificar e preparar a entrega

- Testar migration sobre banco local com dados sintéticos existentes, incluindo contas desativadas e vínculos de pacientes.
- Atualizar testes de domínio, persistência, middleware, handlers e OpenAPI.
- Atualizar documentação e instruções locais que ainda descrevem `POST/PUT /me` ou identidade dentro da conta.
- Regenerar sqlc exclusivamente pelo gerador e publicar o OpenAPI pelo pipeline existente, identificado pelo SHA da API.
- Documentar para web/mobile: trocar POST/PUT por PATCH, ler `profile`, usar `onboarding_completed` e tornar CPF/telefone opcionais.

## Testes de aceitação

- Primeiro acesso autenticado cria uma conta mínima; acessos repetidos ou simultâneos preservam o UUID.
- Issuers diferentes com o mesmo subject não compartilham conta.
- Email igual não vincula identidades; alteração de email não troca o UUID.
- Nome e nascimento concluem o onboarding sem CPF ou telefone.
- PATCH distingue omissão, limpeza e atualização; dados inválidos não produzem alterações parciais.
- CPF preenchido e duplicado retorna 409; várias contas sem CPF são permitidas.
- Perfil pendente bloqueia recursos de negócio; perfil completo mantém as verificações de acesso a pacientes.
- Desativação bloqueia todas as identidades associadas, preserva histórico e impede recriação automática.
- RLS e sua função auxiliar continuam isolando contas e pacientes.
- OpenAPI contém GET/PATCH/DELETE `/me`, sem POST/PUT, com campos opcionais e anuláveis corretamente descritos.
- Executar `go test ./...`, compilação do sqlc e testes de integração com PostgreSQL local. A base atual já passou nos testes Go e na compilação do sqlc.

## Premissas e publicação

- Provisionamento lazy será aplicado às rotas que precisam de conta, sem exigir que o cliente chame `GET /me` antes de qualquer outra operação.
- Nome/avatar vindos do provedor não preencherão automaticamente o perfil nesta entrega.
- Desativar a conta Sonnda não excluirá nem encerrará a identidade no Supabase; não haverá reativação automática.
- Esta entrega altera o contrato dos clientes. Preparar API, migration e documentação agora; publicar em produção junto da adoção do contrato por web/mobile, que está fora do escopo atual.
- Não modificar segredos nem arquivos de `secrets/`; seguir os cabeçalhos de caminho exigidos pelo repositório.
