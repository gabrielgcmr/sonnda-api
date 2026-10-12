<!-- internal/features/documentprocessing/textreading/README.md -->
# Extração textual

Este pacote define o contrato puro para leitura de documentos antes da extração laboratorial estruturada.

`Extractor` recebe a origem local ou remota, o tipo MIME e o nome original. O adaptador retorna o texto bruto, uma versão normalizada para uso semântico e o método utilizado. As implementações concretas ficam em `internal/infrastructure/pdftotext` e `internal/infrastructure/ocr`, sem conhecer HTTP, banco, paciente ou Gemini.

`documentprocessing/labreport` e `documentprocessing/imagingreport` consomem este contrato. A qualidade mínima do texto é avaliada por `IsUsableText`; um PDF sem texto selecionável pode seguir para o extrator multimodal, enquanto a indisponibilidade do leitor continua sendo uma falha técnica.

O fluxo atual usa `pdftotext` para ler PDFs e envia PDFs sem texto legível ao Gemini multimodal. JPEGs e PNGs também seguem diretamente ao Gemini. O adaptador Tesseract fica disponível em `internal/infrastructure/ocr`, mas ainda não está conectado a esses fluxos.
