<!-- docs/architecture/problem-identity-and-access.md -->
# Identidade e vínculo com o paciente — A1.2

Status: contrato documental concluído. A consulta do contexto, o serviço de
confirmação/revogação e a persistência local foram implementados sem endpoint.
A aplicação da política às rotas permanece em A2; as permissões atuais da API
não mudam com esta etapa.

## Decisão de identidade

Um profissional com acesso prévio ao prontuário confirma presencialmente ou por
outro procedimento assistencial apropriado que uma conta registrada pertence à
pessoa daquele paciente. A conta vinculada não pode confirmar a própria identidade
nessa operação, mesmo quando também for profissional. O backend registra o
profissional responsável e o momento da confirmação. O procedimento usado para
conferir a identidade é uma responsabilidade operacional do serviço de saúde;
CPF, nome, data de nascimento e declarações no payload são dados de apoio, não
comprovação automática.

O vínculo confirmado entre conta e paciente é a fonte para reconhecer o **próprio
paciente** nas políticas da A1.1. Ele precisa estar ativo, referir-se exatamente ao
paciente da operação e ter sido estabelecido por profissional que já possuía
acesso àquele prontuário. A confirmação também concede ou mantém o acesso da
conta àquele paciente, na mesma operação. Não torna a conta profissional nem
concede acesso a outros pacientes. A conta vinculada deve existir no backend;
se já houver outra identidade confirmada para o prontuário, a substituição
exige revogação explícita e nova confirmação, com ambas as autorias preservadas.
A repetição exata de uma confirmação ativa não cria outra identidade nem outro
evento de concessão.

O campo `patients.owner_user_id` e o relacionamento
`patient_access.relation_type = 'self'` existentes não são, isoladamente,
comprovação: o fluxo atual de criação
aceita `relation_type` informado pelo cliente e, quando recebe `self`, define
`owner_user_id` como a conta criadora. Registros anteriores à confirmação por
profissional mantêm o acesso que já possuem, mas não recebem a permissão nova de
resolver problemas como próprio paciente. Podem ser confirmados depois pelo
profissional, sem presumir sua identidade a partir do cadastro antigo.

## Contexto consultado pela autorização

Para cada par `(account_id, patient_id)`, a autorização consulta dados persistidos
no backend e obtém:

- conta autenticada e registrada, com `account_type` atual;
- existência do paciente e acesso válido segundo `patient/access`;
- vínculo de identidade confirmado e ativo, se houver;
- vínculo de cuidador concedido por profissional e ativo, se houver;
- demais vínculos ativos de acesso e seu `relation_type`, apenas como contexto.

O resultado distingue `self_verified`, `caregiver_authorized` e acesso comum.
Esses atributos não são papéis globais da conta: devem ser consultados para o
paciente de cada operação. Uma conta `professional` com acesso pode executar as
ações profissionais mesmo se também for paciente ou cuidadora naquele
prontuário. Uma conta `basic_care` não recebe permissão de resolver apenas por seu
tipo. `family` e `professional` em `relation_type` não substituem o tipo da conta
persistido nem comprovam identidade.

`patient/access` continua decidindo se existe acesso. A consulta ao contexto
devolve fatos sobre vínculos e sua proveniência; `authz` combina esses fatos com
a conta para decidir a ação. A condição clínica de problema agudo pertence a
`patient/problem` e não é inferida pelo contexto de acesso. Nenhum papel ou
relação declarado no request concede permissão.

## Revogação, ausência e inconsistências

- Vínculo de identidade ou de cuidador revogado deixa de autorizar novas
  resoluções. A revogação não apaga autoria nem eventos anteriores.
- Se a conta conservar outra forma independente de acesso, ela pode continuar
  lendo conforme a regra atual, sem recuperar a permissão revogada de resolver.
- Se não houver confirmação de identidade nem concessão válida de cuidador, o
  ator é tratado como outro vinculado para ações novas, ainda que haja um
  `relation_type` legado `self` ou `caregiver`.
- Paciente inexistente/inacessível, conta ausente, vínculo ausente/revogado ou
  dados contraditórios não produzem permissão adicional. Falha de consulta não
  é convertida em ausência de vínculo: a operação falha com erro técnico.
- Uma confirmação de identidade não pode apontar simultaneamente para contas
  distintas como o próprio paciente. Conflitos entre confirmação e
  `owner_user_id` ou `relation_type` legados exigem reconciliação por
  profissional; não se escolhe silenciosamente um dos registros. A revogação
  da identidade confirmada retira sua permissão de resolver, mesmo se
  `owner_user_id` ainda conceder leitura pelo checker atual.
- Uma confirmação não pode ser criada a partir de `owner_user_id`, `granted_by`
  ou `relation_type` históricos sem nova verificação e registro de autoria.

O registro da confirmação permite auditar quem confirmou, quando, a qual
conta e paciente se referia, e quem revogou ou substituiu o vínculo. A escrita
é atômica com a concessão de acesso e não sobrescreve silenciosamente
um vínculo de outro tipo. A1.5 revisará as interfaces e definirá o endpoint
de confirmação/revogação. A gestão de
cuidador, inclusive vínculos antigos e conflitos entre relações, será detalhada
na A1.4.

## Exemplos de aceitação

| Contexto | Resultado para a nova permissão de resolver problema agudo |
| --- | --- |
| Conta `basic_care` com identidade confirmada e ativa por outro profissional com acesso | Pode ser reconhecida como próprio paciente, se também mantiver acesso |
| Conta `professional` com identidade confirmada no seu prontuário | Pode agir como paciente ou usar suas permissões profissionais, conforme a ação e seu acesso |
| Conta com `owner_user_id` e `self` criados pelo fluxo antigo, sem confirmação | Não é reconhecida como próprio paciente |
| Criador de prontuário de outra pessoa, com vínculo `family` | Não é reconhecido como próprio paciente |
| Conta com CPF ou nome igual ao cadastro do paciente, sem confirmação | Não é reconhecida como próprio paciente |
| Conta com `caregiver` legado, sem concessão profissional verificável | Não é reconhecida como cuidador autorizado para a nova ação |
| Cuidador com concessão profissional ativa para outro paciente | Não é autorizado neste prontuário |
| Conta com confirmação revogada e outro acesso ativo | Mantém a leitura permitida por esse acesso, sem autorização como próprio paciente |
| Profissional sem acesso ao paciente | Não pode confirmar identidade nem agir no prontuário |
| Conta tenta confirmar a própria identidade | Negado, inclusive se a conta for profissional |
| Consulta ao vínculo falha | Erro técnico; nenhuma permissão nova é concedida |

## Critério de conclusão da A1.2

A identidade do próprio paciente depende de confirmação por profissional com
acesso prévio, com autoria e vigência registradas. A consulta diferencia essa
identidade de vínculo de cuidador e de acesso comum, sem confiar nos valores
enviados pelo cliente ou promover automaticamente registros antigos. A1.4
define a gestão de cuidador; A1.5 define as interfaces e o endpoint de
confirmação/revogação de identidade; A2 implementa as operações e a política.

Plano geral: [Plano de implementação de problemas](problem-implementation-plan.md).
