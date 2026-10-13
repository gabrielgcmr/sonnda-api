<!-- docs/plans/temporary-extractions-implementation-plan.md -->
# Persistência e revisão de extrações temporárias

## Objetivo

O médico poderá sair de `/home/exams`, fechar o navegador e voltar para encontrar os arquivos, o andamento e o resultado durante 24 horas. Fotos laboratoriais poderão formar um trabalho de até 20 páginas. A interface mostrará o documento e os dados extraídos lado a lado.

## Etapa 1 — Persistência e recuperação

**Status:** implementada nos repositórios. A migration deve ser aplicada antes da publicação da API.

- Criar no backend trabalhos temporários associados à conta, com tipo, páginas ordenadas, resultado estruturado e prazo de expiração.
- Referenciar capturas existentes; enviar arquivos escolhidos no computador ao bucket privado temporário.
- Adaptar `/home/exams` para criar, listar e reabrir trabalhos após navegação ou reinício do navegador. A extração continua síncrona nesta etapa.
- Manter os endpoints atuais durante a transição.

## Etapa 2 — Processamento durável

- Enfileirar trabalhos no PostgreSQL e processá-los em worker Go com reserva temporária e retomada após reinício.
- Persistir estado e progresso; permitir nova tentativa enquanto as fontes estiverem disponíveis.
- Confirmar disponibilidade e prazo das páginas antes de gravar o resultado.

## Etapa 3 — Documento e resultado lado a lado

- Mostrar imagem ou PDF com navegação por páginas à esquerda e resultado estruturado à direita; em telas estreitas, alternar entre as áreas.
- Emitir URLs temporárias somente após verificar a conta. Mostrar progresso, falha, prazo e aviso de revisão clínica.
- Limpar a seleção e recarregar os trabalhos ao mudar de conta.

## Etapa 4 — Otimização e múltiplas páginas laboratoriais

- Comprimir fotos no celular e no computador antes do envio, corrigindo orientação e testando inicialmente 2.400 px no lado maior e JPEG de qualidade 0,85. Conservar o original quando a conversão piorar a leitura ou não reduzir tamanho. Preservar PDFs.
- Aceitar de 1 a 20 imagens por trabalho laboratorial. Processar em grupos de até quatro páginas e 10 MiB por chamada, salvar progresso e consolidar resultados com origem por página.
- Preservar resultados de datas ou valores distintos e sinalizar divergências de identificação para revisão. Manter o limite atual de laudos de imagem.

## Contratos e retenção

- Endpoints autenticados: upload temporário; criação, listagem, consulta, nova tentativa e exclusão de trabalhos; acesso temporário às páginas.
- A consulta detalhada retorna tipo, estado, páginas, `expires_at` e resultado no formato já usado pela interface.
- Cada captura pertence a um trabalho ativo. O prazo do trabalho é o vencimento mais próximo entre suas páginas. Capturas mantêm a regra atual de expiração; uploads diretos vencem 24 horas após envio bem-sucedido.
- A exclusão manual do trabalho exclui todas as capturas e uploads diretos associados. Excluir captura pela caixa global invalida seu trabalho. Falhas de remoção do Storage permanecem indisponíveis e são retomadas na limpeza agendada existente.
- Arquivos e resultados não ficam no `localStorage`. Todas as operações e URLs exigem verificação da conta no backend. A inclusão no prontuário continua separada.

## Verificação

- Recuperar resultado após fechar e reabrir navegador; na etapa 2, recuperar também processamento em andamento.
- Mostrar JPEG, PNG e PDF com o resultado e respeitar a ordem e o vencimento das páginas.
- Testar 20 fotos laboratoriais, consolidação com origem, divergências e precisão antes/depois da compressão.
- Testar exclusão, expiração, falha de Storage, retomada do worker e isolamento entre contas.
- Aplicar migrations antes da API e publicar o frontend depois. Executar `go test ./...` e `bun run validate`.
