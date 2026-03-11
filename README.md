# BUFFS ETL Worker

Worker ETL em Go para importação e exportação de planilhas Excel do sistema BUFFS. Atua como um **serviço interno** chamado exclusivamente pela BUFFS API — não expõe interface para usuários finais.

## Arquitetura

```
BUFFS API (NestJS)
     │
     │ X-Internal-Key
     ▼
ETL Worker (Go + Chi)
     │
     ├── Import: Excel → Pipeline ETL → PostgreSQL (COPY)
     └── Export: PostgreSQL → Excel (.xlsx)
```

**Filosofia:** Worker burro e rápido. Toda lógica de negócio, autenticação de usuários e permissões ficam na BUFFS API. O worker apenas processa dados.

## Stack

| Componente | Tecnologia |
|---|---|
| HTTP | `net/http` + Chi v5 |
| Config | `godotenv` + `os.Getenv` |
| Auth | `X-Internal-Key` (inter-serviço) |
| DB | pgx/v5 (COPY protocol) |
| Excel | excelize/v2 |
| Logs | zap (structured) |
| Jobs assíncronos | goroutine + sync.Map (in-memory) |

## Endpoints

### Import (POST, multipart/form-data)
| Rota | Domínio | Tabela |
|---|---|---|
| `POST /import/leite` | Pesagem de leite | `dadoslactacao` |
| `POST /import/pesagem` | Pesagem animal | `dadoszootecnicos` |
| `POST /import/reproducao` | Reprodução | `dadosreproducao` |

**Query params:** `propriedadeId` (obrigatório), `userId` (obrigatório)

### Export (GET, retorna .xlsx)
| Rota | Domínio |
|---|---|
| `GET /export/leite` | Pesagem de leite |
| `GET /export/pesagem` | Pesagem animal |
| `GET /export/reproducao` | Reprodução |

**Filtros:** `propriedadeId` (obrigatório), `grupoId`, `maturidade`, `sexo`, `tipo`, `de`, `ate`, `include_ref`

### Outros
| Rota | Descrição |
|---|---|
| `GET /health` | Healthcheck (público) |
| `GET /jobs/{id}/status` | Status de job assíncrono |

## Pipeline ETL

```
Excel → Extractor → Mapper → Validator → Transformer → Loader (COPY)
```

1. **Extractor:** Lê planilha Excel (aba DADOS ou primeira aba)
2. **Mapper:** Normaliza cabeçalhos (case, acentos, espaços) → nomes internos
3. **Validator:** Valida tipos, ranges, enums, resolução de brincos
4. **Transformer:** Converte strings → structs tipadas do domínio
5. **Loader:** Bulk insert via `pgx.CopyFrom` em transação

## Processamento Assíncrono

Arquivos com mais de `BUFFS_ETL_ASYNC_THRESHOLD` linhas (default: 1000) são processados em background via goroutine. O cliente recebe um `job_id` (HTTP 202) e pode consultar o status em `GET /jobs/{id}/status`.

## Setup

```bash
# 1. Copiar variáveis de ambiente
cp .env.example .env

# 2. Configurar BUFFS_ETL_DB_URL e BUFFS_ETL_INTERNAL_KEY

# 3. Rodar
make dev
```

## Variáveis de Ambiente

| Variável | Descrição | Default |
|---|---|---|
| `BUFFS_ETL_PORT` | Porta HTTP | `8081` |
| `BUFFS_ETL_INTERNAL_KEY` | Chave inter-serviço | — |
| `BUFFS_ETL_DB_URL` | PostgreSQL connection string | `postgresql://postgres:postgres@localhost:5432/buffs_db` |
| `BUFFS_ETL_DB_MAX_CONNS` | Conexões máximas | `20` |
| `BUFFS_ETL_DB_MIN_CONNS` | Conexões mínimas | `5` |
| `BUFFS_ETL_MAX_FILE_SIZE` | Tamanho máximo de upload (bytes) | `52428800` (50MB) |
| `BUFFS_ETL_TEMP_DIR` | Diretório temporário | `./temp/uploads` |
| `BUFFS_ETL_ASYNC_THRESHOLD` | Linhas para processamento assíncrono | `1000` |
| `BUFFS_ETL_INCLUDE_REF_SHEET` | Incluir aba ANIMAIS_REF por padrão | `true` |

## Estrutura

```
cmd/server/main.go          # Entrypoint
internal/
  config/                    # Configuração via .env
  domain/                    # Entidades (MilkRecord, WeightRecord, ReproductionRecord)
  dto/                       # Request/Response DTOs
  extractor/                 # Leitura de Excel
  exporter/                  # Geração de Excel para export
  http/
    handler/                 # Handlers HTTP (import, export, health, job)
    middleware/               # Auth (X-Internal-Key), request logging
    router.go                # Chi router com todas as rotas
  job/                       # Job store in-memory (goroutine)
  loader/                    # PostgreSQL COPY loader
  mapper/                    # Normalização de cabeçalhos Excel
  pipeline/                  # Orquestração Extract→Map→Validate→Transform→Load
  transformer/               # String→Struct tipado
  validator/                 # Validação por linha
pkg/
  apperror/                  # Tipos de erro padronizados
  logger/                    # zap logger factory
```
