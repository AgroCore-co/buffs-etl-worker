# BUFFS ETL Worker

Worker assíncrono em Go para processamento de planilhas Excel de dados pecuários.

## 🎯 Visão Geral

O `buffs-etl-worker` é um microsserviço responsável por resolver um gargalo de performance na API principal (NestJS) do projeto BUFFS. Produtores rurais precisam fazer upload de planilhas Excel com centenas de registros (búfalos, vacinas, pesagens, etc.), e processar esse volume na thread principal bloqueava o Event Loop.

### Problema

```
Cliente → NestJS recebe .xlsx → Parse na thread principal → Bloqueia Event Loop ❌
```

### Solução

```
Cliente → NestJS salva .xlsx → Publica no RabbitMQ → Worker consome e processa ✅
```

## 🏗️ Arquitetura

Implementada com **Clean Architecture** (Hexagonal Architecture):

```
internal/
├── domain/               # Entidades do domínio (sem deps externas)
│   └── message.go        # ExcelProcessingMessage
├── port/                 # Interfaces (portas de entrada/saída)
│   └── consumer.go       # MessageConsumer interface
├── config/               # Configurações centralizadas
│   └── config.go         # Variáveis de ambiente
└── infrastructure/       # Adaptadores concretos
    └── messaging/        # Detalhes de implementação
        └── rabbitmq_consumer.go
cmd/
└── worker/
    └── main.go           # Entrypoint com graceful shutdown
```

### Fluxo de Consumo

1. **NestJS publica** mensagem JSON na fila `excel_processing_queue`
2. **Worker consome** via RabbitMQ (biblioteca `amqp091-go`)
3. **Console handler** desserializa o payload
4. **Use Case** processa a planilha (parse Excel + bulk insert)
5. **ACK/NACK** confirma ou reenfileira a mensagem

### Resiliência

- ✅ Reconexão automática com retry exponencial
- ✅ Prefetch = 1 (processa uma mensagem por vez)
- ✅ ACK manual (controle total de confirmação)
- ✅ Graceful shutdown → captura SIGINT/SIGTERM

## 🚀 Primeiros Passos

### Pré-requisitos

- Go 1.25+
- RabbitMQ rodando
- Docker + Docker Compose (para modo containerizado)

### Instalação Local (Development)

```bash
# 1. Clonar repositório
git clone https://github.com/AgroCore-co/buffs-etl-worker.git
cd buffs-etl-worker

# 2. Configurar variáveis de ambiente
cp .env.example .env
# Edite .env para configurar:
# - RABBITMQ_URL (credenciais do RabbitMQ)
# - DATABASE_URL (opcional - para resolução brinco→UUID)
# - UPLOAD_BASE_PATH (caminho dos uploads da API)

# 3. Instalar dependências Go
go mod download
go mod tidy

# 4. Se RabbitMQ não estiver rodando, subir via Docker:
docker run -d \
  --name rabbitmq \
  -p 5672:5672 \
  -p 15672:15672 \
  -e RABBITMQ_DEFAULT_USER=admin \
  -e RABBITMQ_DEFAULT_PASS=admin \
  rabbitmq:3.13-management-alpine

# 5. Executar o worker
# O arquivo .env é carregado automaticamente via godotenv
go run ./cmd/worker/main.go
```

> **💡 Nota:** O worker agora carrega o arquivo `.env` automaticamente na inicialização usando `godotenv`. Em produção/Docker, as variáveis são injetadas diretamente pelo container.

### Instalação com Docker (Production-ready)

#### Opção A: Worker + API Principal Juntos (Recomendado)

```bash
# 1. Subir API principal primeiro
cd buffs-api
docker-compose -f infra/docker-compose.yml up -d

# 2. Subir o worker (conecta no RabbitMQ da API via network interna)
cd ../buffs-etl-worker
docker-compose up -d

# 3. Verificar logs
docker-compose logs -f buffs-worker

# 4. Parar tudo
docker-compose down
```

#### Opção B: Worker Standalone com RabbitMQ Remoto

```bash
# Editar .env com RABBITMQ_URL para um servidor remoto
export RABBITMQ_URL="amqp://admin:admin@rabbitmq-server.com:5672/"

# Build e run
docker build -t buffs-etl-worker .
docker run -d \
  -e RABBITMQ_URL="$RABBITMQ_URL" \
  -e RABBITMQ_QUEUE="excel_processing_queue" \
  --name buffs-worker \
  buffs-etl-worker
```

## 🔧 Variáveis de Ambiente

Crie um arquivo `.env` na raiz do projeto baseado em `.env.example`:

```bash
cp .env.example .env
# Edite .env com suas credenciais reais
```

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | Connection string AMQP. Use `rabbitmq:5672` como host quando rodando via Docker (network interna) |
| `RABBITMQ_QUEUE` | `excel_processing_queue` | Nome da fila a consumir |
| `UPLOAD_BASE_PATH` | `../buffs-api/temp/uploads` | Diretório onde os arquivos Excel são salvos pela API. Em Docker: `/shared/uploads` |
| `DATABASE_URL` | *(vazio)* | Connection string PostgreSQL para resolução `brinco→UUID`. Se vazio, abas dependentes (Pesagens, Sanitário) não processam FKs |

### DATABASE_URL (Opcional)

O worker pode resolver FKs de brinco para UUID consultando o PostgreSQL:

**Supabase:**
```env
DATABASE_URL=postgresql://postgres:[PASSWORD]@db.[PROJECT_ID].supabase.co:5432/postgres
```

**PostgreSQL Local:**
```env
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/buffs_db
```

**PostgreSQL via Docker:**
```env
DATABASE_URL=postgresql://postgres:postgres@postgres:5432/buffs_db
```

> **⚠️ Importante:** Se `DATABASE_URL` não for configurado, abas como **Pesagens**, **Sanitário**, **Reprodução** não conseguirão processar porque dependem da resolução `brinco → id_bufalo`. Apenas a aba **Animais** funcionará (ela insere os búfalos diretamente).

### Credenciais

As credenciais RabbitMQ são definidas no `docker-compose.yml` da API principal:

```yaml
environment:
  RABBITMQ_DEFAULT_USER: admin    # ← use isso
  RABBITMQ_DEFAULT_PASS: admin    # ← use isso
```

**Para modo Docker (recomendado):**
```env
RABBITMQ_URL=amqp://admin:admin@rabbitmq:5672/
```

**Para modo Local (dev):**
```env
RABBITMQ_URL=amqp://admin:admin@localhost:5672/
```

## 📋 Payload Esperado

O NestJS publica mensagens JSON neste formato:

```json
{
  "file_path": "/tmp/planilha_fazenda_x.xlsx",
  "farm_id": "123-abc",
  "user_id": "456-def"
}
```

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `file_path` | string | Caminho local ou S3 do arquivo .xlsx |
| `farm_id` | string | ID único da fazenda |
| `user_id` | string | ID do usuário que realizou upload |

## 🧪 Testar Localmente

### 1. Rodar Infrastructure (RabbitMQ via API Docker Compose)

```bash
# Usar o docker-compose da API principal
cd buffs-api
docker-compose -f infra/docker-compose.yml up -d rabbitmq

# Aguarde ~40s para RabbitMQ estar pronto
docker-compose -f infra/docker-compose.yml logs -f rabbitmq | grep "Server startup complete"
```

Acesse dashboard RabbitMQ: http://localhost:15672 (admin:admin)

### 2. Iniciar o Worker

**Opção A: Via Go (desenvolvimento)**
```bash
cd buffs-etl-worker
export RABBITMQ_URL="amqp://admin:admin@localhost:5672/"
go run ./cmd/worker/main.go
```

**Opção B: Via Docker (production-like)**
```bash
cd buffs-etl-worker
docker-compose up -d

# Ver logs
docker-compose logs -f buffs-worker
```

### 3. Publicar Mensagem de Teste

```bash
# Via amqp-utils (Linux)
echo '{"file_path":"/tmp/test.xlsx","farm_id":"farm-1","user_id":"user-1"}' | \
  amqp-publish -u admin -p admin -H localhost -e excel_processing_queue -b

# Ou usar RabbitMQ Admin UI (Browser)
# 1. Ir em http://localhost:15672
# 2. Queue → excel_processing_queue → Publish Message
# 3. Copiar/colar o JSON acima
```

### 4. Observar Consumer Processar

```bash
# Esperado no terminal do worker:
# [RabbitMQ] Conexão estabelecida com sucesso
# [RabbitMQ] Consumer ativo | Aguardando mensagens na fila 'excel_processing_queue'...
# [RabbitMQ] Mensagem recebida | FarmID: farm-1 | UserID: user-1 | Arquivo: /tmp/test.xlsx
# [Handler] Processando planilha: /tmp/test.xlsx | Fazenda: farm-1 | Usuário: user-1
# [RabbitMQ] Mensagem processada com sucesso | FarmID: farm-1
```

## 🐛 Troubleshooting

### Erro: "username or password not allowed"

**Causa:** Credenciais RabbitMQ incorretas na `RABBITMQ_URL`.

**Solução:**
```bash
# Verifique as credenciais do docker-compose da API
cat ../buffs-api/infra/docker-compose.yml | grep RABBITMQ_DEFAULT

# Sua .env deve usar as mesmas credenciais:
RABBITMQ_URL=amqp://admin:admin@localhost:5672/
```

### Erro: "Cannot connect to RabbitMQ"

**Causa:** RabbitMQ não está rodando ou não está acessível.

**Solução:**
```bash
# Verificar se RabbitMQ está rodando
docker ps | grep rabbitmq

# Testar conexão
docker exec buffs-rabbitmq rabbitmq-diagnostics ping
# Esperado: "pong"

# Ver logs de erro
docker-compose -f infra/docker-compose.yml logs rabbitmq
```

### Erro: "Network not found" (ao rodar em Docker)

**Causa:** O worker está tentando se conectar mas a network do docker-compose da API não existe.

**Solução:**
```bash
# Subir a API PRIMEIRO para criar a network
cd buffs-api
docker-compose -f infra/docker-compose.yml up -d

# Depois subir o worker
cd ../buffs-etl-worker
docker-compose up -d
```

### Worker processa mas recebe erro do handler

**Causa:** Handler retorna erro (implementação futura).

**Comportamento esperado:**
```
[RabbitMQ] Erro ao processar mensagem (reenfileirando): <erro>
# Mensagem volta para a fila e será reprocessada
```

Isso é normal durante desenvolvimento da lógica de ETL.

## 📚 Próximas Fases

### Fase 2: Use Cases de ETL
- [ ] Criar `usecase/process_excel.go` com lógica de parse (Excelize)
- [ ] Implementar `domain/Animal`, `domain/Vaccine`, `domain/Weighing`
- [ ] Criar repository para bulk insert no PostgreSQL

### Fase 3: Persistência
- [ ] Conectar PostgreSQL via SQLC ou Ent
- [ ] Implementar transações para garantir consistência
- [ ] Adicionar logging estruturado (Slog ou Zap)

### Fase 4: Observabilidade
- [ ] Integrar OpenTelemetry
- [ ] Métricas Prometheus (mensagens processadas, latência, erros)
- [ ] Tracing distribuído

### Fase 5: Deploy
- [ ] Dockerfile multistage
- [ ] Kubernetes manifests
- [ ] CI/CD pipeline

## 📖 Referências

- [RabbitMQ Go Client](https://pkg.go.dev/github.com/rabbitmq/amqp091-go)
- [Excelize Documentation](https://xuri.me/excelize/)
- [Clean Architecture in Go](https://pkg.go.dev/github.com/bxcodec/go-clean-arch)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)

## 📝 Licença

Mesmo projeto pai (BUFFS).

---

**Desenvolvido com ❤️ para a comunidade pecuária brasileira.**
