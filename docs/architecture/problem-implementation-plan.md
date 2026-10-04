<!-- docs/architecture/problem-implementation-plan.md -->
# Plano de implementação de problemas do paciente

Este documento organiza a implementação futura da API em duas partes:
autorização e lógica dos problemas. Não altera contratos HTTP, código ou banco.
Cada etapa deve resultar em uma entrega pequena e verificável.

## Regras já decididas

- Problema vinculado ao paciente, compartilhado entre profissionais com acesso.
- Nome livre obrigatório; sem catálogo ou detecção/aviso automático de duplicidade.
- CID-11 opcional, preservando código, sistema e versão quando informado.
- Situação clínica: `active` ou `resolved`; criação inicialmente ativa.
- Cronicidade representada por um campo simples, definido somente por profissionais.
- Somente profissionais criam problemas, editam nome/CID, unificam, retificam
  e reabrem problemas. Paciente não pode criar problemas.
- O próprio paciente ou seu cuidador autorizado pode resolver apenas problemas
  explicitamente classificados como agudos pelo profissional; nenhum deles pode
  classificar problemas por conta própria.
- A única alteração permitida ao paciente e ao cuidador é marcar como resolvido.
  Leitura segue as regras de acesso existentes. A resolução registra o autor real,
  distinguindo paciente, cuidador e profissional, e o paciente ao qual se refere.
- Problemas crônicos não podem ser resolvidos, nem por profissionais.
- Na unificação, o profissional escolhe nome livre, CID, situação clínica e
  cronicidade finais. Não há herança automática dessas escolhas.
- Retificação de registro indevido exige motivo e preserva auditoria.
- Unificação e retificação são condições administrativas, não resolução clínica.
- Nesta fase inicial, uma conta registrada pode ser habilitada como profissional
  mediante uma senha de habilitação validada exclusivamente pelo backend.
- Somente profissionais com acesso ao paciente podem conceder ou revogar seu
  vínculo de cuidador; essa ação não concede acesso profissional a outros pacientes.

## Contrato proposto para cronicidade

Usar um único campo `is_chronic`, sem catálogo de classificações:

| Valor | Significado | Paciente/cuidador autorizado pode resolver? |
| --- | --- | --- |
| `true` | Crônico | Não |
| `false` | Agudo, classificado explicitamente pelo profissional | Sim |
| `null` | Ainda não classificado | Não |

A representação anulável é uma proposta de implementação para distinguir ausência
de classificação de classificação explícita. Não usar `false` como padrão, pois
isso permitiria resolução pelo paciente/cuidador sem classificação profissional.
Como a criação é exclusiva de profissionais, se a classificação for obrigatória
na criação, um booleano obrigatório será suficiente. A obrigatoriedade ainda
precisa ser definida; classificação ausente nunca libera resolução pelo paciente/cuidador.

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
  `basic_care` pode representar paciente ou cuidador. O vínculo com cada paciente
  possui metadados `self`, `caregiver`, `family` ou `professional`, que atualmente
  não concedem permissões por ação.
- Encontros/evoluções e outros vínculos clínicos serão integrados quando existirem.

## Parte A — Autorização

### A1 — Definição dos contratos de autorização

A1 fica dividida em cinco subetapas. Cada uma fecha um contrato pequeno e seus
critérios de aceitação. A implementação fica em A2; testes pertinentes acompanham
cada entrega e A3 verifica o conjunto.

#### A1.1 — Matriz de ações e permissões

Status: **concluída como contrato documental**. Matriz codificada em `authz` e
testada de forma isolada; integração das políticas com dados confiáveis e
endpoints permanece em A2.

Entrega: [Permissões de problemas do paciente](problem-permissions.md), contendo:

- Matriz consolidada de ações para profissional, paciente, cuidador e outros vinculados.
- Condições de acesso e regras específicas de resolução, reabertura, retificação,
  unificação e concessão/reativação/revogação de cuidador.
- Separação entre autorização por ação e validação clínica do problema.
- Exemplos de aceitação para operações permitidas e negadas, incluindo vínculo
  revogado, ausência de classificação e ações em prontuários diferentes.

As definições de identidade e consulta ao vínculo ficam na A1.2. O contrato da
senha de habilitação fica na A1.3; gestão de vínculos na A1.4; interfaces na A1.5.

#### A1.2 — Identidade e vínculo com o paciente

Status: **concluída como contrato documental**. Implementação das consultas e
operações em A2, após definição das interfaces em A1.5.

- Entrega: [Identidade e vínculo com o paciente](problem-identity-and-access.md).
- Manter os tipos de conta `professional` e `basic_care`.
- Reconhecer o próprio paciente somente por vínculo confirmado por outro
  profissional com acesso prévio, com autoria e vigência registradas. Criador,
  `owner_user_id` e `self` legados não comprovam identidade por si sós.
- Formalizar como recuperar o vínculo ativo e sua relação (`self`, `caregiver`,
  `family`, `professional`) e a proveniência da confirmação/concessão, pois o
  checker atual informa apenas se há acesso.
- A confirmação profissional concede acesso ao paciente na mesma operação;
  revogação e substituição preservam autoria e não promovem registros antigos.
- Definir tratamento de ausência/revogação do vínculo, conta profissional que
  também é paciente/cuidador e dados inconsistentes.
- Preservar a checagem de acesso para profissionais; a condição profissional
  não concede acesso automático a prontuários.

Entrega: contrato de consulta ao contexto de acesso, com dados provenientes
exclusivamente do backend. Nenhuma permissão vem de um papel enviado no payload.

#### A1.3 — Habilitação profissional por senha

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

Entrega: contrato do endpoint, configuração e critérios de aceitação, preparado
para implementação em `account` durante A2.

#### A1.4 — Concessão e revogação de cuidador

- Formalizar `PUT /patients/{patientId}/caregivers/{accountId}` para conceder ou
  reativar vínculo de cuidador de uma conta já cadastrada.
- Formalizar `DELETE /patients/{patientId}/caregivers/{accountId}` para revogação.
- Exigir profissional com acesso prévio ao paciente em ambas as operações;
  impedir autoatribuição de acesso por essa operação.
- Registrar quem concedeu/revogou e quando; revogação impede resoluções posteriores.
- Definir repetição das operações e o caso de vínculo existente de outro tipo,
  preservando identidades e relações sem sobrescrita ou revogação acidental.
- Não estender resolução automaticamente a familiares, qualquer conta `basic_care`
  ou qualquer vínculo ativo.

Entrega: contratos HTTP e transições do vínculo, preparados para implementação
em `patient/access`, com política aplicada por `authz` durante A2.

#### A1.5 — Limites arquiteturais e integração

- `account`: habilitação e persistência do tipo da conta.
- `patient/access`: consulta, concessão e revogação dos vínculos.
- `authz`: decisões por ação a partir de conta confiável e contexto do paciente.
- `patient/problem`: regras clínicas e mudanças de estado, incluindo cronicidade,
  com auditoria; implementação durante a Parte B.
- Definir as interfaces entre os serviços e sua composição no bootstrap.
- Definir endpoint e persistência da confirmação/revogação de identidade,
  preservando os outros vínculos de acesso e o histórico de autoria.
- Definir erros e observabilidade seguindo `AppError`/`humaerror` existentes.
- Conferir os contratos de A1.1–A1.4 em conjunto, sem mover regras clínicas para
  autenticação ou para o checker de acesso.

Entrega: interfaces propostas, fluxo de chamadas e contratos consolidados.
A1 está concluída quando A2 puder ser implementada sem decisões implícitas sobre
quem pode agir, como obter seu contexto e quais dados cada operação aceita.

### A2 — Políticas por ação

- Implementar uma política pequena em `authz`, conforme as instruções do repositório,
  reutilizando `RequireAccess` e mantendo autorização fora de `patient/access`.
- Reconhecer profissionais por dados confiáveis da conta; não aceitar papel
  profissional no payload nem em metadados editáveis pelo usuário.
- Autorizar criação, edição, classificação, reabertura, retificação e unificação somente
  a profissionais com acesso.
- Autorizar resolução pelo próprio paciente ou cuidador autorizado daquele paciente,
  sujeita à regra clínica de agudo e ao vínculo ativo no momento da operação.
- Implementar habilitação profissional em `account` e gerenciamento de cuidadores
  em `patient/access`, com as rotas e os contratos definidos em A1.
- Autorizar concessão/revogação de cuidador exclusivamente a profissional com
  acesso prévio ao paciente; impedir autoatribuição de acesso por essa operação.
- Aplicar a permissão de classificação também na criação e unificação.
- Definir uma interface consumível pela feature de problemas, sem exigir que
  suas tabelas ou endpoints já existam.

Conclusão: política verificável de forma independente, com entradas e negativas
explícitas. A regra de cronicidade continua pertencendo ao domínio de problemas.

### A3 — Verificação da autorização

- Testar profissional com e sem acesso, próprio paciente, cuidador autorizado,
  cuidador com vínculo revogado e outros vinculados. Familiar com acesso não recebe
  automaticamente a permissão de cuidador.
- Verificar que um paciente que também seja profissional pode exercer permissões
  profissionais quando sua classificação e acesso forem válidos.
- Testar falhas de consulta e ausência de identidade, sem concessão de permissões.
- Testar habilitação com senha válida/inválida, limite de tentativas e configuração
  ausente; verificar que a senha não aparece em respostas ou observabilidade.
- Testar concessão/revogação por profissional com acesso e negativa para profissional
  sem acesso, paciente, cuidador ou familiar; preservar autoria e outros vínculos.
- Usar o contrato central `AppError`; consumidores HTTP traduzem com `humaerror`.

Conclusão: Parte A pronta para ser integrada, sem liberar novas ações clínicas
nos endpoints existentes.

## Parte B — Lógica dos problemas

### B1 — Domínio, persistência e auditoria

- Definir problema, situação clínica, cronicidade e condição administrativa
  (`valid`, `merged`, `entered_in_error`). Os nomes são propostas de contrato.
- Criar problemas e histórico com autoria, timestamps, versão, valores anteriores/
  novos e motivo quando aplicável; cada mudança e seu evento ficam na mesma transação.
- Criar migration em `supabase/migrations`, schemas/queries do sqlc e adaptador
  PostgreSQL; gerar arquivos pelo tooling, sem edição manual dos gerados.
- Impedir crônico resolvido na aplicação e na persistência.
- Permitir nomes/CIDs repetidos: duplicidade clínica é decisão do usuário.
- Manter tabelas protegidas, inclusive via RLS em schemas expostos, sem acesso
  direto do cliente que contorne regras da API.

Conclusão: invariantes e persistência verificadas; base pronta para os endpoints.

### B2 — Criação e consultas

- Implementar criação, listagem paginada, detalhe e consulta ao histórico.
- Aplicar autorização da Parte A: apenas profissionais com acesso podem criar.
- Aceitar nome livre e CID opcional, iniciando como ativo.
- Receber classificação profissional conforme o contrato final de B1;
  se não informada e permitida, não assumir que o problema é agudo.
- Listar registros válidos por padrão e oferecer filtros para situação e histórico
  administrativo; conectar bootstrap e rotas Huma.

Conclusão: problemas podem ser criados e consultados por usuários autorizados,
com autoria e histórico preservados.

### B3 — Edição, classificação, resolução e reabertura

- Permitir a profissionais editar nome/CID e cronicidade, mantendo ID e histórico.
- Paciente ou cuidador autorizado resolve somente quando `is_chronic` for
  explicitamente `false`; registrar a identidade real de quem realizou a operação.
- Reabertura exclusiva de profissionais; resolução de crônicos proibida para todos.
- Impedir mudança de resolvido para crônico sem reabertura válida; validar o
  estado final de requests que alterem mais de um campo.
- Proteger mudanças com controle de versão e transações; conferir a classificação
  atual ao resolver, evitando corrida com uma alteração profissional de cronicidade.

Conclusão: transições corretas e autoria registrada; requests simultâneos não
contornam a proibição de crônico resolvido nem sobrescrevem alterações.

### B4 — Retificação por engano

- Operação explícita, exclusiva de profissionais, com motivo não vazio obrigatório.
- Marcar registro indevido e retirá-lo da listagem padrão sem exclusão física.
- Preservar conteúdo e histórico; impedir alterações clínicas comuns posteriores.

Conclusão: motivo e autoria recuperáveis; registro indevido não aparece como resolvido.

### B5 — Unificação manual

- Operação exclusiva de profissionais com destino e origens do mesmo paciente.
- Exigir nome final livre e escolhas explícitas de CID, situação e cronicidade,
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

- Testar fluxos completos com múltiplos profissionais, paciente, cuidador autorizado
  e outros vinculados, incluindo a autoria da resolução pelo cuidador.
- Testar rollback, concorrência e repetição de requests de resolução/unificação.
- Validar migrations em banco descartável e operações transacionais em PostgreSQL.
- Gerar/compilar sqlc com a configuração vigente e executar checks do repositório.
- Conferir OpenAPI Huma e documentar payloads para o mobile.
- Entregar banco antes da API dependente e verificar no ambiente de teste antes
  da publicação dos endpoints.

Conclusão: Parte B integrada à autorização, contrato coerente e regras confirmadas
verificadas. Testes pertinentes acompanham cada etapa, não ficam todos para B6.

## Ordem e divisão das entregas

A1.1 → A1.2 → A1.3 → A1.4 → A1.5 → A2 → A3 → B1 → B2 → B3 → B4 → B5 → B6.
A1 pode ser entregue em incrementos documentais no mesmo PR. Para implementação,
uma entrega/PR por etapa; separar domínio e persistência em B1 se ficar grande.
O primeiro incremento utilizável chega ao final de B3. O escopo completo inclui B4 e B5.

Pendências de contrato: representação da escolha sem CID na unificação.
A forma anulável ou obrigatória do booleano depende da obrigatoriedade de
classificação na criação e é proposta técnica.

Interface mobile, notificações semanais, catálogo de terminologia, consulta externa
CID-11 e implementação de encontros/evoluções/prescrições ficam fora deste plano.
