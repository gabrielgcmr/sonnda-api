
Este README referencia os ADRs quando necessário, mas **não substitui** esses registros.

---

## ADRs

- `docs/architecture/adr/ADR-001-repositorio-com-valor-vs-ponteiro.md`
- `docs/architecture/adr/ADR-002-error-handling-contrato.md`
- `docs/architecture/adr/ADR-003-estratégia-auth.md`
- `docs/architecture/adr/ADR-004-parser-deterministico-exames-laboratoriais.md`
- `docs/architecture/adr/ADR-005-extracao-semantica-exames-laboratoriais.md`


---


## Quando criar um novo ADR

Um novo ADR deve ser criado quando:

- a decisão impactar múltiplos módulos
- a escolha não for óbvia para um novo contribuidor
- existirem alternativas plausíveis
- a decisão puder ser questionada no futuro

Exemplos comuns:
- ponteiros vs valores
- DTO vs entidade
- organização por feature vs camada
- contratos HTTP
- estratégias de autenticação/autorização

---

## Observação final

Este documento descreve **como a arquitetura está organizada**.

Os ADRs explicam **por que ela é assim**.

Sempre que uma decisão arquitetural relevante for tomada,  
o código e a documentação devem evoluir juntos.
