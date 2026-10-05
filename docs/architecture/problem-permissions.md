<!-- docs/architecture/problem-permissions.md -->
# Permissões de problemas do paciente — A1.1

Status: contrato revisado pela A1.4. A matriz está codificada em
`internal/features/authz/problem_policy.go`, ainda sem ligação aos endpoints.

## Contexto de autorização

Para qualquer operação sobre problemas, o backend exige uma conta registrada e
acesso ativo ao paciente da operação. O `relation_type` do vínculo é metadado e
não participa da decisão. Não há distinção de permissão entre paciente, cuidador,
familiar ou outro vínculo.

O `account_type` continua separando as ações profissionais. Uma conta
`professional` não ganha acesso automático a pacientes. Uma conta `basic_care`
com acesso ativo pode consultar e resolver problema agudo, mas não pode executar
ações profissionais.

## Matriz de ações

| Ação | `professional` com acesso | `basic_care` com acesso |
| --- | --- | --- |
| Listar/consultar problemas e histórico | Sim | Sim |
| Criar problema | Sim | Não |
| Editar nome/CID | Sim | Não |
| Definir/alterar cronicidade | Sim | Não |
| Marcar como resolvido | Sim, respeitando regras clínicas | Sim, somente agudo |
| Reabrir problema | Sim | Não |
| Retificar registro por engano | Sim | Não |
| Unificar problemas | Sim | Não |

## Condições das ações

- A resolução exige `is_chronic = false`. Ausência de classificação não autoriza
  a resolução.
- Problema crônico não pode ser resolvido por nenhuma conta.
- Reabertura, criação, edição, classificação, retificação e unificação são ações
  exclusivas de conta `professional` com acesso ativo.
- Retificação exige motivo e preserva conteúdo e histórico.
- Unificação exige problemas do mesmo paciente, escolhas finais explícitas e
  rejeita o resultado crônico e resolvido.
- A autorização é avaliada para o paciente de cada operação. Acesso em um
  prontuário não concede acesso em outro.

O histórico registra a conta que executou a ação e o paciente afetado. Não é
necessário classificar o autor como paciente, cuidador ou familiar para justificar
a autorização.

## Responsabilidades

- `patient/access` decide se a conta possui acesso ativo ao paciente.
- `authz` combina o acesso com o `account_type` persistido para decidir a ação.
- `patient/problem` valida cronicidade, transições, retificação e unificação.
- `account` fornece o tipo persistido e a habilitação profissional da A1.3.

Plano geral: [Plano de implementação de problemas](problem-implementation-plan.md).
