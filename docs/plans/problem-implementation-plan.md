<!-- problem-implementation-plan.md -->
# Plano de implementação de problemas do paciente

Este documento organiza a implementação futura da API em duas partes:
autorização e lógica dos problemas. Não altera contratos HTTP, código ou banco.
Cada etapa deve resultar em uma entrega pequena e verificável.

## Regras já decididas

- Problema vinculado ao paciente, compartilhado entre profissionais com acesso.
- Nome livre obrigatório; sem catálogo ou detecção/aviso automático de duplicidade.
- CID-11 opcional, preservando código, sistema e versão quando informado.
- Situação clínica: `active` ou `resolved`; criação inicialmente ativa.
- Classificação clínica do problema definida somente por profissionais; nesta fase,
  os valores são `acute` e `chronic`.
- Somente profissionais criam problemas, editam nome/CID, unificam, retificam
  e reabrem problemas. Paciente não pode criar problemas.
- Qualquer conta registrada com acesso ativo pode resolver apenas problemas
  explicitamente classificados como agudos pelo profissional. `relation_type`
  e a relação da conta com o paciente não participam dessa autorização.
- A única alteração permitida a `basic_care` é marcar como resolvido. Leitura
  segue as regras de acesso existentes. A resolução registra a conta autora e o
  paciente ao qual se refere.
- Problemas crônicos não podem ser resolvidos, nem por profissionais.
- Na unificação, o profissional escolhe nome livre, CID, situação clínica e
  classificação finais. Não há herança automática dessas escolhas.
- Retificação de registro indevido exige motivo e preserva auditoria.
- Unificação e retificação são condições administrativas, não resolução clínica.
- Nesta fase inicial, uma conta registrada pode ser habilitada como profissional
  mediante uma senha de habilitação validada exclusivamente pelo backend.

## Contrato de classificação clínica

Usar `classification` com valores explícitos, sem booleano `is_chronic`:

| Valor | Significado | Conta com acesso pode resolver? |
| --- | --- | --- |
| `acute` | Agudo, classificado explicitamente pelo profissional | Sim |
| `chronic` | Crônico | Não |

Somente `acute` libera resolução. Não atribuir `acute` como padrão, pois isso
permitiria resolução sem classificação profissional. A classificação é obrigatória
na criação por profissional; ausência e valores desconhecidos são rejeitados.
`latent` pode ser estudado no futuro, inclusive quanto a ser uma classificação
exclusiva ou uma dimensão clínica separada; não faz parte do contrato atual.

## Estrutura existente e dependências

- REST em Go, Gin e Huma; registros Huma definem o OpenAPI.
- Feature proposta: `internal/features/patient/problem`, com domínio, aplicação,
  HTTP e adaptador PostgreSQL seguindo as features atuais.
- Composição em `internal/application/bootstrap` e `internal/api/routes.go`.
- Queries e schemas do sqlc em `internal/infrastructure/database/postgres/sqlc`.
- Migrations vigentes em `supabase/migrations`; as migrations antigas dentro
  da infraestrutura estão deprecadas.
- `patient/access` verifica vínculo; autorização por ação deve ficar separada.
- `AccountTypeProfessional` existe, mas o cadastro HTTP atual cria `basic_care`.
  A habilitação inicial usará senha validada no backend e atualizará o tipo da conta.
- Os tipos atuais são `professional` e `basic_care`; não há tipo de conta `patient`.
  O vínculo com cada paciente possui metadados `self`, `caregiver`, `family` ou
  `professional`, sem efeito sobre permissões por ação.
- Encontros/evoluções e outros vínculos clínicos serão integrados quando existirem.

## Parte A — Autorização

### A1 — Definição dos contratos de autorização

A1 fica dividida em cinco subetapas. Cada uma fecha um contrato pequeno e seus
critérios de aceitação. A implementação fica em A2; testes pertinentes acompanham
cada entrega e A3 verifica o conjunto.

#### A1.1 — Matriz de ações e permissões

Status: **concluída e revisada pela A1.4**. Matriz codificada em `authz` e
testada de forma isolada; integração das políticas com dados confiáveis e
endpoints permanece em A2.

Entrega: [Contrato de autorização](docs/architecture/authz/README.md), contendo:

- Matriz consolidada para contas `professional` e `basic_care` com acesso.
- Condições de acesso e regras específicas de resolução, reabertura, retificação,
  unificação e resolução.
- Separação entre autorização por ação e validação clínica do problema.
- Exemplos de aceitação para operações permitidas e negadas, incluindo vínculo
  revogado, ausência de classificação e ações em prontuários diferentes.

O contexto de conta e acesso fica na A1.2. O contrato da senha de habilitação
fica na A1.3; a simplificação da autorização, na A1.4; interfaces, na A1.5.

#### A1.2 — Contexto de conta e acesso

Status: **revisada pela A1.4**. O contrato de identidade confirmada foi revogado;
o contexto agora depende somente da conta registrada e do acesso ativo.

- Entrega: [Contrato de autorização](docs/architecture/authz/README.md).
- Manter os tipos de conta `professional` e `basic_care`.
- Consultar o `account_type` persistido e exigir acesso ativo ao paciente.
- Manter `relation_type` como metadado sem efeito autorizador.
- Preservar a checagem de acesso para profissionais; a condição profissional
  não concede acesso automático a prontuários.

Entrega: contrato de consulta ao contexto de acesso, com dados provenientes
exclusivamente do backend. Nenhuma permissão vem de um papel enviado no payload.

#### A1.3 — Habilitação profissional por senha

Status: **concluída no contrato e no código**. O endpoint Huma, a validação bcrypt,
a persistência em `account` e o limite distribuído de tentativas com Redis estão
implementados e testados.

Entrega: [Habilitação profissional por senha](docs/architecture/account/professional-activation.md).

- Formalizar `POST /me/professional-activation` para a própria conta autenticada
  e registrada; validar senha e persistir `account_type=professional`.
- Manter senha de habilitação separada da senha de login.
- Configurar `PROFESSIONAL_ACTIVATION_PASSWORD_HASH`: `.env` local fora do Git e
  variável secreta no ambiente publicado. `.env.example` contém apenas o nome.
- Comparar a senha recebida com hash bcrypt; não incluir senha/hash em aplicativo,
  repositório, respostas, logs ou histórico. Não gerar valor real nesta etapa.
- Definir limites de tentativas por conta e origem, configuração ausente/inválida,
  senha incorreta e repetição da ativação de uma conta já profissional.
- Registrar a ativação sem guardar a senha; a autorização posterior consulta
  a condição profissional persistida. Ativação não concede vínculos com pacientes.
- Documentar que trocar o hash afeta novas ativações e não rebaixa contas existentes.

Entrega: contrato e implementação do endpoint, configuração e critérios de
aceitação. A autorização posterior consulta o tipo persistido da conta.

#### A1.4 — Simplificação da autorização por acesso

Status: **concluída no contrato e no código de autorização**.

- Revogar o fluxo especial de identidade própria e cuidador autorizado.
- Permitir resolução de problema agudo para qualquer conta com acesso ativo.
- Remover identidade, condição de cuidador e `relation_type` do contexto de
  autorização.
- Remover o serviço e o adaptador de confirmação de identidade própria.
- Preservar a migration já criada no histórico; retirar a tabela física somente
  por migration compensatória após verificar os ambientes e a retenção dos dados.

Entrega: [Contrato de autorização](docs/architecture/authz/README.md).

#### A1.5 — Limites arquiteturais e integração

Status: **concluída no contrato e no código de autorização**.

- `account`: habilitação e persistência do tipo da conta.
- `patient/access`: consulta do acesso e manutenção dos vínculos existentes.
- `authz`: decisões por ação a partir de conta confiável e contexto do paciente.
- `patient/problem`: regras clínicas e mudanças de estado, incluindo classificação,
  com auditoria; implementação durante a Parte B.
- Definir as interfaces entre os serviços e sua composição no bootstrap.
- Definir erros e observabilidade seguindo `AppError`/`humaerror` existentes.
- Conferir os contratos de A1.1–A1.4 em conjunto, sem mover regras clínicas para
  autenticação ou para o checker de acesso.

Entrega: [Contrato de autorização](docs/architecture/authz/README.md),
com `authz.ProblemAuthorizer` como interface única consumida pela futura feature
de problemas. A1 está concluída: A2 pode ser implementada sem decisões implícitas
sobre quem pode agir ou como obter seu contexto.

### A2 — Políticas por ação

Status: **concluída no código**.

- Política implementada em `authz`, reutilizando `RequireAccess` e mantendo a
  autorização por ação fora de `patient/access`.
- Profissionais são reconhecidos por dados confiáveis da conta; papéis informados
  no payload ou em metadados editáveis pelo usuário não são aceitos.
- Criação, edição, classificação, reabertura, retificação e unificação são
  autorizadas somente para profissionais com acesso.
- Resolução é autorizada para qualquer conta registrada com acesso ativo, sujeita à
  regra clínica de problema agudo no momento da operação.
- A habilitação profissional implementada em `account` fornece o tipo persistido
  consultado pela política.
- A permissão de classificação também cobre criação e unificação.
- `authz.ProblemAuthorizer` fornece a interface consumível pela futura feature de
  problemas, sem depender de suas tabelas ou endpoints.

Conclusão: política implementada e verificável de forma independente, com entradas
e negativas explícitas. A regra de classificação continua pertencendo ao domínio de
problemas. A A3 fará a verificação consolidada da autorização.

### A3 — Verificação da autorização

Status: **concluída nos testes automatizados**.

- Contas `professional` e `basic_care` foram verificadas com e sem acesso,
  inclusive para paciente divergente e ausência de vínculo ativo.
- O contexto de autorização não recebe `relation_type`; `account_type` continua
  restringindo as ações profissionais.
- Falhas de consulta, identidade ausente, ação desconhecida e dependências não
  configuradas foram verificadas sem concessão de permissão.
- A habilitação profissional cobre senha válida e inválida, limite de tentativas,
  configuração ausente ou inválida e falha do limitador.
- Os erros usam o contrato `AppError`; consumidores HTTP traduzem com `humaerror`.

Conclusão: Parte A verificada e pronta para integração, sem liberar novas ações
clínicas nos endpoints existentes. Os testes HTTP das operações de problemas
serão adicionados com os endpoints das etapas B2 e B3.

## Parte B — Lógica dos problemas

### B1 — Domínio, persistência e auditoria

#### B1.1 — Contrato e regras do domínio

Status: **concluída no domínio e nos testes**. A classificação `classification`
é obrigatória na criação, com valores `acute` e `chronic`.

- Definir o problema, a situação clínica (`active`, `resolved`), a classificação
  (`acute`, `chronic`) e a condição administrativa (`valid`, `merged`,
  `entered_in_error`).
- Rejeitar criação sem classificação explícita.
- Modelar autoria, timestamps, versão, valores anteriores/novos e motivo quando
  aplicável no histórico. Definir as transições e invariantes de domínio que
  serão aplicados às futuras operações.
- Impedir a combinação `chronic` + `resolved`; permitir nomes e CIDs repetidos,
  pois a avaliação de duplicidade clínica pertence ao usuário.
- Testar as regras do domínio, inclusive ausência e valor desconhecido de
  classificação, resolução, reabertura e condições administrativas.

Conclusão: contrato e invariantes definidos e testados em
`internal/features/patient/problem/domain`, sem dependência do banco. A
autorização profissional continua em `authz`; B1.2 ainda precisa garantir
versões e eventos na mesma transação PostgreSQL.

#### B1.2 — Persistência transacional e auditoria

Status: **concluída na migration, sqlc e adaptador PostgreSQL**. Criação e
alteração usam transações que gravam a versão do problema e seu evento de
histórico em conjunto; alterações exigem a versão anterior esperada.

- Criar migration em `supabase/migrations` para problemas e histórico, com
  autoria, timestamps, versão, valores anteriores/novos e motivo quando aplicável.
- Criar schemas/queries do sqlc e adaptador PostgreSQL; gerar arquivos pelo
  tooling, sem edição manual dos gerados.
- Gravar cada alteração e seu evento de histórico na mesma transação; preparar
  controle de versão e operações atômicas para as etapas seguintes.
- Reforçar no banco a proibição de `chronic` + `resolved`, conforme o contrato
  de B1.1, sem impor unicidade a nome ou CID.
- Manter tabelas protegidas, inclusive via RLS em schemas expostos, sem acesso
  direto do cliente que contorne regras da API.
- Verificar migration, restrições e rollback das operações transacionais em
  PostgreSQL descartável; compilar a geração do sqlc.

Conclusão: persistência e auditoria verificadas em PostgreSQL descartável,
incluindo rollback, conflito de versão, restrição de crônico resolvido,
duplicidades permitidas, RLS, privilégios e histórico imutável. Base pronta
para os endpoints de B2.

### B2 — Criação e consultas

Status: **concluída nos serviços, consultas PostgreSQL e endpoints Huma**.
Criação, listagem, detalhe e histórico usam a autorização da Parte A e estão
conectados ao bootstrap. A listagem usa `limit`/`offset`, `has_more`, filtros
`clinical_status` e `administrative_status`, com `valid` por padrão.
Histórico paginado por versão decrescente preserva snapshots e autoria.
Contrato e exemplos: `internal/features/patient/problem/README.md`.
A execução requer a migration de B1.2 aplicada no ambiente.

- Implementar criação, listagem paginada, detalhe e consulta ao histórico.
- Aplicar autorização da Parte A: apenas profissionais com acesso podem criar.
- Aceitar nome livre e CID opcional, iniciando como ativo.
- Exigir classificação profissional `acute` ou `chronic` na criação.
- Listar registros válidos por padrão e oferecer filtros para situação e histórico
  administrativo; conectar bootstrap e rotas Huma.

Conclusão: problemas podem ser criados e consultados por usuários autorizados,
com autoria e histórico preservados.

### B3 — Edição, classificação, resolução e reabertura

Status: **concluída nos serviços e endpoints Huma**. Edição de nome/CID,
classificação, resolução e reabertura são operações separadas, exigindo a
versão atual no payload. Usam as transições do domínio de B1.1 e a atualização
transacional com auditoria de B1.2; conflitos retornam `409`.
Contrato e exemplos: `internal/features/patient/problem/README.md`.
Testes HTTP cobrem permissões, regras clínicas, autoria, CID, conflitos e
mudança de classificação entre leitura e escrita. Testes PostgreSQL cobrem
operações simultâneas, mas exigem `PROBLEMS_TEST_DATABASE_URL` local para execução.
Verificação desta entrega: suíte Go e exportação OpenAPI executadas com sucesso;
testes de integração PostgreSQL pendentes por ausência de instância local configurada.

- Permitir a profissionais editar nome/CID e classificação, mantendo ID e histórico.
- Qualquer conta com acesso ativo resolve somente quando a classificação for
  explicitamente `acute`; registrar a conta que realizou a operação.
- Reabertura exclusiva de profissionais; resolução de crônicos proibida para todos.
- Impedir mudança de resolvido para crônico sem reabertura válida; validar o
  estado final de requests que alterem mais de um campo.
- Proteger mudanças com controle de versão e transações; conferir a classificação
  atual ao resolver, evitando corrida com uma alteração profissional da classificação.

Conclusão: transições corretas e autoria registrada; requests simultâneos não
contornam a proibição de crônico resolvido nem sobrescrevem alterações.

### B4 — Retificação por engano

Status: **concluída nos serviços e endpoint Huma**. A operação
`POST /patients/{patientId}/problems/{problemId}/rectify` exige profissional com
acesso, versão atual e motivo não vazio. A atualização preserva os campos clínicos,
marca o registro como `entered_in_error` e grava autoria, motivo e snapshots na
mesma transação. Conflitos retornam `409`; registros retificados deixam a listagem
padrão e continuam consultáveis por detalhe, histórico e filtro administrativo.

- Operação explícita, exclusiva de profissionais, com motivo não vazio obrigatório.
- Marcar registro indevido e retirá-lo da listagem padrão sem exclusão física.
- Preservar conteúdo e histórico; impedir alterações clínicas comuns posteriores.

Conclusão: motivo e autoria recuperáveis; registro indevido não aparece como resolvido.

### B5 — Unificação manual

- Operação exclusiva de profissionais com destino e origens do mesmo paciente.
- Exigir nome final livre e escolhas explícitas de CID, situação e classificação,
  mesmo se os valores dos registros originais forem iguais.
- Como CID é opcional, definir a representação da escolha final sem código,
  distinguindo escolha explícita de omissão no payload.
- Validar registros válidos, impedir auto-unificação, origens repetidas e ciclos.
- Rejeitar escolha final de crônico resolvido.
- Atualizar destino, marcar origens e gravar eventos numa única transação protegida
  contra concorrência; registrar `merged_into_id` sem apagar IDs/históricos.
- Permitir recuperar os históricos incorporados e suas origens, inclusive após
  unificações sucessivas. Integrar vínculos clínicos quando esses recursos existirem.

Conclusão: falhas não deixam unificação parcial; escolhas finais são respeitadas
sem perder autoria, nomes, códigos ou estados anteriores.

### B6 — Validação integrada e entrega

- Testar fluxos completos com contas `professional` e `basic_care`, diferentes
  tipos de vínculo e autoria da resolução.
- Testar rollback, concorrência e repetição de requests de resolução/unificação.
- Validar migrations em banco descartável e operações transacionais em PostgreSQL.
- Gerar/compilar sqlc com a configuração vigente e executar checks do repositório.
- Conferir OpenAPI Huma e documentar payloads para o mobile.
- Entregar banco antes da API dependente e verificar no ambiente de teste antes
  da publicação dos endpoints.

Conclusão: Parte B integrada à autorização, contrato coerente e regras confirmadas
verificadas. Testes pertinentes acompanham cada etapa, não ficam todos para B6.

## Ordem e divisão das entregas

A1.1 → A1.2 → A1.3 → A1.4 → A1.5 → A2 → A3 → B1.1 → B1.2 → B2 → B3 → B4 → B5 → B6.
A1 pode ser entregue em incrementos documentais no mesmo PR. Para implementação,
uma entrega/PR por etapa; B1.1 fecha o domínio antes da migration de B1.2.
O primeiro incremento utilizável chega ao final de B3. O escopo completo inclui B4 e B5.

Pendências de contrato: representação da escolha sem CID na unificação.

Interface mobile, notificações semanais, catálogo de terminologia, consulta externa
CID-11 e implementação de encontros/evoluções/prescrições ficam fora deste plano.
