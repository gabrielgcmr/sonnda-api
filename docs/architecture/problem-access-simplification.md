<!-- docs/architecture/problem-access-simplification.md -->
# Simplificação da autorização por acesso — A1.4

Status: concluída no contrato e no código de autorização.

Esta etapa substitui a proposta anterior de gestão especial de cuidador e desfaz
a lógica de identidade criada para esse fluxo.

## Alterações

- Qualquer conta registrada com acesso ativo ao paciente pode resolver um
  problema agudo.
- `self`, `caregiver`, `family` e `professional` em `relation_type` não alteram
  permissões.
- `account_type = professional` continua obrigatório para criar, editar,
  classificar, reabrir, retificar e unificar problemas.
- O contexto de autorização carrega somente conta, paciente, tipo da conta e
  confirmação de acesso.
- O serviço e o adaptador de confirmação de identidade própria foram removidos.
- As ações especiais de conceder e revogar cuidador foram removidas da política
  de problemas.

## Persistência anterior

A migration `20261004185751_patient_self_confirmations.sql` não é apagada nem
alterada, pois ela pode integrar o histórico aplicado de algum ambiente. A tabela
não participa mais da autorização. Sua remoção física exige uma migration nova e
uma verificação prévia dos ambientes e da política de retenção dos registros.

## Critério de conclusão

Os testes devem demonstrar que uma conta `basic_care` com acesso ativo pode
resolver, que a ausência de acesso bloqueia a ação e que as ações profissionais
continuam restritas a `professional` com acesso.
