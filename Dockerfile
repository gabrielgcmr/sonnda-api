# Dockerfile
# Etapa 1: build
FROM golang:1.26-alpine3.24 AS build
WORKDIR /app
ARG VERSION=0.1.0

RUN apk add --no-cache ca-certificates tzdata
# Copia go.mod, go.sum e baixa dependências
COPY go.mod go.sum ./
RUN go mod download

# Copia
COPY . .

#Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w -X github.com/gabrielgcmr/sonnda/cmd/api.version=${VERSION}" \
  -o /bin/sonnda ./cmd/api

# Etapa 2: runtime
FROM alpine:3.24
WORKDIR /app

# poppler-utils fornece pdftotext, usado pela extracao de texto de PDFs.
RUN apk add --no-cache ca-certificates poppler-utils tzdata

COPY --from=build /bin/sonnda /app/sonnda

# Expõe a porta
ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/sonnda"]
