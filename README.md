# ETL BUFFS

Microsserviço em Go para importação e exportação de dados agropecuários via planilhas Excel (`.xlsx`), integrado à plataforma **BUFFS** — sistema de gestão de propriedades rurais com foco em bovinos.

## 🎯 Visão Geral

Produtores rurais muitas vezes **não têm tempo nem mão de obra** para inserir dados históricos manualmente. O ETL resolve isso:

- **Import**: produtor preenche uma planilha `.xlsx` padronizada → sistema faz bulk insert no banco via `pgx COPY`
- **Export**: sistema gera planilha `.xlsx` pré-preenchida com os animais da propriedade para facilitar o preenchimento

## ⚙️ Stack Tecnológica

| Camada | Tecnologia |
|---|---|
| Linguagem | Go 1.25+ |
| HTTP | Fiber v2 |
| Excel | `excelize/v2` |
| Banco de Dados | PostgreSQL via `pgx/v5` (COPY protocol) |
| Jobs Assíncronos | `asynq` + Redis |
| Config | `viper` + `.env` |
| Logs | `zap` (structured logging) |
| Métricas | Prometheus |
| Auth | JWT Bearer Token (Supabase — mesmo da BUFFS API) |

## 🏗️ Arquitetura

Implementada com **Clean Architecture** (pipeline ETL):

```
cmd/
└── server/
    └── main.go               # Entrypoint: HTTP + Asynq worker + graceful shutdown
internal/
├── config/                    # Viper: .env e defaults
├── domain/                    # Structs puras de negócio
│   ├── common.go              # Record interface, ImportResult, BrincoLookup, Animal
│   ├── milk.go                # MilkRecord (tabela dadoslactacao)
│   ├── weight.go              # WeightRecord (tabela dadoszootecnicos)
│   └── reproduction.go        # ReproductionRecord (tabela dadosreproducao)
├── dto/                       # Request/Response DTOs (HTTP)
├── extractor/                 # Leitura do Excel (excelize)
├── mapper/                    # Cabeçalho Excel ↔ campo DB (case-insensitive, sem acento)
├── validator/                 # Regras de negócio (brinco existe? data válida? range?)
├── transformer/               # Normalização, conversão de tipos, resolução de FKs
├── loader/                    # Bulk Insert via pgx COPY + BrincoLookup
├── exporter/                  # Geração de .xlsx a partir do banco
├── pipeline/                  # Orquestração: Extract → Map → Validate → Transform → Load
├── job/                       # Workers Asynq (processamento assíncrono)
└── http/
    ├── router.go              # Fiber app com todos os endpoints
    ├── handler/               # Import, Export, Job, Health handlers
    └── middleware/             # JWT auth, rate limiting, request logger
pkg/
├── apperror/                  # AppError com Code, Message, Row
└── logger/                    # zap.Logger factory
```

### Fluxo de Import (Excel → Banco)

```
POST /import/{type} com .xlsx
  → Extractor (excelize: lê aba "Dados" linha a linha)
  → Mapper (cabeçalho Excel ↔ campo interno, case-insensitive, sem acento)
  → Validator (tipos, ranges, existência de brincos via batch query)
  → Transformer (normaliza datas, trata nulos, resolve brinco→UUID)
  → Loader (pgx COPY INTO → bulk insert em transação)
  → Response JSON { total_rows, imported, skipped, errors[], warnings[] }
```

### Fluxo de Export (Banco → Excel)

```
GET /export/{type}?filtros
  → Query Builder (filtros → SQL parametrizado)
  → Fetch (pgx streaming)
  → Exporter (excelize gera .xlsx com estilos e validações)
  → Injeta aba ANIMAIS_REF (lista de brincos disponíveis)
  → Response: arquivo .xlsx como download
```

### Processamento Assíncrono

Arquivos com mais de **1.000 linhas** (configurável via `BUFFS_ETL_ASYNC_THRESHOLD`) são enfileirados automaticamente:

```
POST /import/{type} → 202 Accepted { job_id }
  → Worker Asynq processa em background
  → GET /jobs/{id}/status → { status, progress }
```

## 📊 Domínios de Dados

### 1. Pesagem do Leite (`/import/milk` | `/export/milk`)

| Campo Excel | Obrigatório | Observação |
|---|---|---|
| Brinco | SIM | Deve existir na propriedade |
| Data | SIM | DD/MM/AAAA, sem datas futuras |
| Qtd. Produzida (L) | SIM | >= 0, warning acima de 60L |
| Turno | NÃO | AM / PM / Único |
| Escore CCS | NÃO | Escala 0-9 |
| Observação | NÃO | Máx. 500 chars |

### 2. Pesagem do Animal (`/import/weight` | `/export/weight`)

| Campo Excel | Obrigatório | Observação |
|---|---|---|
| Brinco | SIM | Deve existir na propriedade |
| Data | SIM | DD/MM/AAAA, sem datas futuras |
| Peso (kg) | SIM | > 0, warning acima de 1500kg |
| Método | NÃO | Balança / Fita / Estimativa Visual |
| Escore Corporal (BCS) | NÃO | 1.0 a 5.0 |
| Observação | NÃO | Máx. 500 chars |

### 3. Reprodução (`/import/reproduction` | `/export/reproduction`)

| Campo Excel | Obrigatório | Observação |
|---|---|---|
| Brinco Fêmea | SIM | Deve ser fêmea |
| Brinco Macho | NÃO | Obrigatório para MN |
| Data Evento | SIM | Data da IA, cobertura ou TE |
| Tipo | SIM | MN / IA / IATF / TE |
| Cód. Material Genético | NÃO | Sêmen ou embrião |
| Resultado DG | NÃO | Positivo / Negativo / Pendente / Inconclusivo |
| Observação | NÃO | Máx. 500 chars |

## 🌐 Endpoints REST

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/import/milk` | Importa planilha de pesagem do leite |
| `POST` | `/import/weight` | Importa planilha de pesagem do animal |
| `POST` | `/import/reproduction` | Importa planilha de reprodução |
| `GET` | `/export/milk` | Exporta planilha de pesagem do leite |
| `GET` | `/export/weight` | Exporta planilha de pesagem do animal |
| `GET` | `/export/reproduction` | Exporta planilha de reprodução |
| `GET` | `/template/milk` | Baixa template vazio de leite |
| `GET` | `/template/weight` | Baixa template vazio de pesagem |
| `GET` | `/template/reproduction` | Baixa template vazio de reprodução |
| `GET` | `/jobs/:id/status` | Status de job assíncrono |
| `GET` | `/health` | Healthcheck |
| `GET` | `/metrics` | Métricas Prometheus |

**Query params de export:** `property_id` (obrigatório), `group_id`, `maturity`, `sex`, `from`, `to`, `include_ref`

**Autenticação:** todos os endpoints (exceto `/health` e `/metrics`) exigem `Authorization: Bearer <JWT>`.

## 🚨 Tratamento de Erros

| Tipo | Comportamento |
|---|---|
| Brinco não encontrado | Erro por linha, continua o import |
| Data inválida | Erro por linha, continua o import |
| Valor suspeito (fora do range) | Warning, importa com flag |
| Duplicata exata | Skip + warning |
| Coluna obrigatória ausente | **Erro fatal** — abort total |
| Arquivo corrompido / não-xlsx | **Erro fatal** — abort total |

**Exemplo de resposta do import:**
```json
{
  "job_id": "uuid-se-async",
  "total_rows": 250,
  "imported": 241,
  "skipped": 5,
  "errors": [
    { "row": 14, "field": "brinco", "value": "#9999", "message": "Brinco não encontrado na propriedade" }
  ],
  "warnings": [
    { "row": 88, "field": "qtd_litros", "value": "9999", "message": "Valor acima do esperado para produção diária" }
  ]
}
```

## 🚀 Primeiros Passos

### Pré-requisitos

- Go 1.25+
- PostgreSQL (mesmo da BUFFS API)
- Redis (para jobs assíncronos via Asynq)
- Docker + Docker Compose (para modo containerizado)

### Instalação Local

```bash
# 1. Clonar repositório
git clone https://github.com/AgroCore-co/buffs-etl-worker.git
cd buffs-etl-worker

# 2. Configurar variáveis de ambiente
cp .env.example .env
# Edite .env com suas credenciais (ver seção abaixo)

# 3. Instalar dependências
go mod download && go mod tidy

# 4. Subir Redis (se não estiver rodando)
docker run -d --name etl-redis -p 6379:6379 redis:7-alpine

# 5. Rodar o servidor
make dev
# ou: go run ./cmd/server
```

### Instalação com Docker

```bash
# 1. Subir a API principal primeiro (para criar a network buffs-network)
cd buffs-api
docker-compose -f infra/docker-compose.yml up -d

# 2. Subir ETL + Redis
cd ../buffs-etl-worker
docker-compose up -d

# 3. Verificar logs
docker-compose logs -f etl-buffs

# 4. Parar tudo
docker-compose down
```

### Comandos Make

```bash
make dev              # Roda em modo desenvolvimento
make build            # Compila binário otimizado em ./bin/
make test             # Roda testes com race detection + coverage
make test-coverage    # Gera relatório HTML de cobertura
make lint             # Roda golangci-lint
make docker-build     # Build da imagem Docker
make docker-up        # Sobe ETL + Redis
make docker-down      # Para tudo
make deps             # go mod tidy + download
make fmt              # Formata código (gofmt + goimports)
```

## 🔧 Variáveis de Ambiente

Todas prefixadas com `BUFFS_ETL_`. Crie `.env` a partir de `.env.example`:

```bash
cp .env.example .env
```

| Variável | Padrão | Descrição |
|---|---|---|
| `BUFFS_ETL_ENV` | `development` | Ambiente (`production` \| `development`) |
| `BUFFS_ETL_PORT` | `3001` | Porta do servidor HTTP |
| `BUFFS_ETL_READ_TIMEOUT` | `30s` | Timeout de leitura HTTP |
| `BUFFS_ETL_WRITE_TIMEOUT` | `60s` | Timeout de escrita HTTP |
| `BUFFS_ETL_DB_URL` | `postgresql://...localhost:5432/buffs_db` | Connection string PostgreSQL |
| `BUFFS_ETL_DB_MAX_CONNS` | `20` | Máximo de conexões no pool |
| `BUFFS_ETL_DB_MIN_CONNS` | `5` | Mínimo de conexões no pool |
| `BUFFS_ETL_DB_MAX_CONN_LIFETIME` | `30m` | Tempo máximo de vida de uma conexão |
| `BUFFS_ETL_REDIS_URL` | `redis://localhost:6379` | URL do Redis (Asynq) |
| `BUFFS_ETL_JWT_SECRET` | *(vazio)* | Secret do JWT Supabase (obrigatório) |
| `BUFFS_ETL_MAX_FILE_SIZE` | `52428800` (50MB) | Tamanho máximo de upload em bytes |
| `BUFFS_ETL_TEMP_DIR` | `./temp/uploads` | Diretório temporário para uploads |
| `BUFFS_ETL_INCLUDE_REF_SHEET` | `true` | Incluir aba ANIMAIS_REF no export |
| `BUFFS_ETL_WORKER_CONCURRENCY` | `5` | Workers Asynq simultâneos |
| `BUFFS_ETL_ASYNC_THRESHOLD` | `1000` | Linhas acima desse valor → job assíncrono |
| `BUFFS_ETL_RATE_LIMIT_IMPORTS_PER_HOUR` | `10` | Máx. imports por hora por propriedade |

### PostgreSQL

O ETL usa o **mesmo banco** da BUFFS API principal. Exemplos:

```env
# Supabase
BUFFS_ETL_DB_URL=postgresql://postgres.xxx:password@aws-1-sa-east-1.pooler.supabase.com:6543/postgres?pgbouncer=true

# PostgreSQL local
BUFFS_ETL_DB_URL=postgresql://postgres:postgres@localhost:5432/buffs_db

# Docker
BUFFS_ETL_DB_URL=postgresql://postgres:postgres@postgres:5432/buffs_db
```

## 🔐 Segurança

- Auth via **JWT Bearer Token** (mesmo da BUFFS API principal / Supabase)
- Autorização por `property_id`: usuário só acessa dados da própria propriedade
- Limite de tamanho de arquivo: **50MB** por upload (configurável)
- Rate limiting: máximo **10 imports/hora** por propriedade
- Graceful shutdown com captura de `SIGINT` / `SIGTERM`

## 🐛 Troubleshooting

### Erro: "Falha ao conectar no PostgreSQL"

```bash
# Verifique se o PostgreSQL está acessível
psql "$BUFFS_ETL_DB_URL" -c "SELECT 1"

# Se estiver usando Supabase, verifique se a senha e o endpoint estão corretos
```

### Erro: "Redis connection refused"

```bash
# Verifique se o Redis está rodando
docker ps | grep redis
redis-cli ping  # esperado: PONG
```

### Erro: "Network not found" (Docker)

```bash
# A network buffs-network é criada pelo docker-compose da API principal
cd buffs-api
docker-compose -f infra/docker-compose.yml up -d

# Depois suba o ETL
cd ../buffs-etl-worker
docker-compose up -d
```

### Erro 401 nos endpoints

Verifique se `BUFFS_ETL_JWT_SECRET` está configurado com o mesmo secret do Supabase usado pela BUFFS API.

## 📖 Referências

- [Fiber Web Framework](https://docs.gofiber.io/)
- [pgx - PostgreSQL Driver](https://pkg.go.dev/github.com/jackc/pgx/v5)
- [Excelize Documentation](https://xuri.me/excelize/)
- [Asynq - Distributed Task Queue](https://pkg.go.dev/github.com/hibiken/asynq)
- [Viper Configuration](https://pkg.go.dev/github.com/spf13/viper)
- [Zap Logger](https://pkg.go.dev/go.uber.org/zap)

## 📝 Licença

Mesmo projeto pai (BUFFS).

---

**Desenvolvido com ❤️ para a comunidade pecuária brasileira.**
