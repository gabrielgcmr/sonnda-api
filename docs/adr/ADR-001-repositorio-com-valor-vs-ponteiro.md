# ADR-005 — Retorno por valor no Repository e por ponteiro no Service

**Status:** Aceito  
**Data:** 2025-08  
**Contexto:** Sonnda API – módulo Patient (padrão aplicado aos demais módulos)

---

## Contexto

Na Sonnda API, a arquitetura segue uma separação clara entre:

- **Domain**: entidades e regras de negócio
- **App**: services de aplicação e políticas de acesso
- **Infrastructure**: persistência e integrações externas

Durante a implementação dos services do módulo Patient, surgiu a necessidade de definir **como as entidades do domínio são retornadas pelos repositórios** e **como elas são manipuladas na camada de aplicação**.

Especificamente:
- Os **services** trabalham com entidades do domínio (`patient.Patient`)
- Os **repositories** retornam dados provenientes do banco (PostgreSQL via sqlc)

A dúvida central era se os repositories deveriam retornar:
- slices de ponteiros (`[]*Patient`)
- ou slices de valores (`[]Patient`)

---

## Decisão

A decisão adotada foi:

### Repository
- Métodos de **listagem** retornam **valores**:
  ```go
  List(...) ([]patient.Patient, error)
