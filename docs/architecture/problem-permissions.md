<!-- docs/architecture/problem-permissions.md -->
# Permissões de problemas do paciente — A1.1

Status: contrato documental concluído. A matriz de atores e ações também está
codificada em `internal/features/authz/problem_policy.go`, ainda sem ligação aos
endpoints. A integração das consultas e das regras clínicas ocorre em A2 e na
Parte B; este documento não altera as permissões atuais da API.

## Atores e contexto

O tipo da conta e sua relação com um paciente são informações diferentes.

- **Profissional com acesso**: conta `professional` reconhecida pelo backend e
  autorizada a acessar o paciente da operação.
- **Próprio paciente**: conta identificada de forma confiável como a pessoa daquele
  prontuário, com acesso válido. Ter criado o cadastro não comprova essa identidade.
- **Cuidador autorizado**: conta com vínculo ativo de cuidador daquele paciente,
  concedido por profissional com acesso.
- **Outro vinculado**: conta com acesso ao paciente, como um familiar, sem condição
  profissional ou permissão de cuidador. O acesso permite leitura, conforme a regra atual.

Uma conta pode ser o próprio paciente de um prontuário e cuidadora em outro.
Uma conta profissional também pode ser paciente ou cuidadora; sua condição
profissional continua permitindo ações profissionais quando houver acesso.
A1.2 define como consultar e validar esses contextos e a confirmação
profissional necessária para reconhecer o próprio paciente.

## Condições comuns

1. A conta precisa estar autenticada e registrada.
2. Toda operação sobre um paciente exige acesso válido àquele paciente.
3. A condição profissional vem da conta persistida, e o vínculo vem do backend.
   Tipo de conta ou relação enviados no payload não concedem permissão.
4. Ser profissional não concede acesso automático a todos os pacientes.
5. Ser `basic_care`, dono do cadastro ou ter um vínculo genérico não autoriza
   automaticamente agir como próprio paciente ou cuidador.
6. Vínculo revogado deixa de conceder suas permissões. Outros acessos independentes,
   quando existentes, são avaliados sem restaurar a permissão revogada.
7. A decisão deve considerar o paciente de cada operação: autorização em um
   prontuário não concede autorização em outro.

A habilitação profissional é uma operação sobre a própria conta, sem paciente
associado. Seu contrato de senha pertence à A1.3 e não amplia acesso a prontuários.

## Matriz de ações

Todos os atores desta tabela já satisfazem as condições comuns de acesso.

| Ação | Profissional com acesso | Próprio paciente | Cuidador autorizado | Outro vinculado |
| --- | --- | --- | --- | --- |
| Listar/consultar problemas e histórico | Sim | Sim | Sim | Conforme acesso atual |
| Criar problema | Sim | Não | Não | Não |
| Editar nome/CID | Sim | Não | Não | Não |
| Definir/alterar cronicidade | Sim | Não | Não | Não |
| Marcar como resolvido | Sim, respeitando regras clínicas | Somente agudo | Somente agudo | Não |
| Reabrir problema | Sim | Não | Não | Não |
| Retificar registro por engano | Sim | Não | Não | Não |
| Unificar problemas | Sim | Não | Não | Não |
| Conceder/reativar vínculo de cuidador | Sim | Não | Não | Não |
| Revogar vínculo de cuidador | Sim | Não | Não | Não |

## Condições específicas das ações

### Resolução e reabertura

- Paciente e cuidador podem realizar apenas a alteração de ativo para resolvido;
  essa permissão não permite editar nome, CID, cronicidade ou outros campos.
- Essa resolução exige classificação explícita como agudo feita por profissional.
  Ausência de classificação não autoriza paciente ou cuidador a resolver.
- Nenhum ator pode resolver um problema crônico, inclusive profissionais.
- Reabertura é exclusiva de profissionais, mesmo quando o paciente ou cuidador
  realizou a resolução original.
- Resolver um problema não encerra nem resolve automaticamente outros problemas.

### Retificação

- Somente profissional com acesso pode marcar um registro como indevido.
- Motivo é obrigatório. Conteúdo e histórico permanecem preservados.
- Retificação por engano não representa resolução clínica.

### Unificação

- Somente profissional com acesso pode unificar problemas.
- Todos os problemas envolvidos devem pertencer ao mesmo paciente.
- O profissional escolhe nome final livre, CID, situação clínica e cronicidade.
  A API não decide esses valores por similaridade ou por herança automática.
- A combinação final crônico e resolvido é proibida.
- Problemas de origem e seus históricos são preservados, identificando a unificação.
- Não há detecção, aviso, bloqueio ou unificação automática por duplicidade.

### Gestão de cuidador

- Concessão, reativação e revogação exigem profissional com acesso prévio ao paciente.
- Paciente, cuidador e familiar sem condição profissional não podem executar essas ações.
- A concessão se refere a uma conta destinatária e a um paciente específicos;
  não transforma a conta em profissional nem autoriza acesso a outros pacientes.
- A revogação impede usar aquele vínculo para futuras resoluções e preserva o
  histórico das ações já realizadas.
- Tratamento de vínculos existentes de outro tipo e repetição das operações
  será definido em A1.4.

## Responsabilidades na arquitetura

- `patient/access` informa acesso e vínculo, preservando suas regras atuais.
- `authz` decide se o ator pode executar a ação naquele paciente.
- `patient/problem` valida condições clínicas e administrativas da operação.
  Permissão profissional não supera a proibição de crônico resolvido.
- `account` fornece a condição profissional persistida e será responsável
  pela habilitação descrita em A1.3.

O histórico registra a identidade real de quem realiza a ação. Uma resolução
feita por cuidador registra o cuidador como autor, com o paciente atendido e o
contexto utilizado para autorizar a operação. Concessão/revogação registra o
profissional responsável. Os contratos de persistência serão definidos nas etapas seguintes.

## Exemplos de aceitação

| Cenário | Resultado esperado |
| --- | --- |
| Profissional com acesso cria um problema com nome livre e sem CID | Permitido |
| Profissional com acesso altera o nome ou CID de problema existente | Permitido, mantendo ID e histórico |
| Paciente com acesso resolve o próprio problema classificado como agudo | Permitido, com autoria do paciente |
| Cuidador ativo resolve um problema agudo do paciente ao qual está vinculado | Permitido, com autoria do cuidador |
| Profissional com acesso reabre um problema resolvido pelo cuidador | Permitido |
| Profissional com acesso concede ou revoga cuidador daquele paciente | Permitido, com autoria do profissional |
| Familiar com acesso consulta os problemas | Permitido conforme regra atual de leitura |
| Profissional sem acesso tenta ler ou alterar o prontuário | Negado |
| Paciente ou cuidador tenta criar, editar nome/CID ou alterar cronicidade | Negado |
| Paciente ou cuidador tenta reabrir, retificar ou unificar | Negado |
| Paciente ou cuidador tenta conceder, reativar ou revogar cuidador | Negado |
| Qualquer ator tenta resolver um problema crônico | Negado pela regra clínica |
| Paciente ou cuidador tenta resolver um problema ainda não classificado | Negado |
| Familiar com acesso, sem condição profissional ou vínculo de cuidador, tenta resolver | Negado |
| Cuidador tenta resolver em outro prontuário sem autorização correspondente | Negado |
| Conta usa somente um vínculo de cuidador revogado para tentar resolver | Negado |
| Request declara que a conta é profissional sem essa condição no backend | Não concede permissão |
| Profissional tenta unificar problemas de pacientes diferentes | Negado |
| Profissional tenta retificar sem motivo | Negado pela validação da operação |
| Profissional unifica escolhendo crônico e resolvido como resultado | Negado pela regra clínica |

## Critério de conclusão da A1.1

A matriz e suas condições estão consolidadas neste documento, com exemplos
permitidos e negados. As etapas A1.2–A1.5 definirão consulta de identidade/vínculo,
contratos de habilitação e gestão de cuidador, e interfaces entre as features.
A obrigatoriedade da classificação na criação e a representação de CID ausente
na unificação permanecem decisões de contrato das etapas correspondentes; não
alteram as permissões acima.

Plano geral: [Plano de implementação de problemas](problem-implementation-plan.md).
