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
- RabbitMQ rodando (use Docker ou instale localmente)
- PostgreSQL (para próximas fases)

### Instalar Dependências

```bash
go mod download
go mod tidy
```

### Rodar o Worker

```bash
# Desenvolvimento (com RabbitMQ em localhost)
go run ./cmd/worker/main.go

# Com variáveis de ambiente customizadas
export RABBITMQ_URL="amqp://user:pass@rabbitmq-host:5672/"
export RABBITMQ_QUEUE="custom_queue"
go run ./cmd/worker/main.go
```

### Compilar para Produção

```bash
go build -o bin/worker ./cmd/worker
./bin/worker
```

## 🔧 Variáveis de Ambiente

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | Connection string AMQP |
| `RABBITMQ_QUEUE` | `excel_processing_queue` | Nome da fila a consumir |

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

### 1. Rodar RabbitMQ com Docker

```bash
docker run -d \
  --name rabbitmq \
  -p 5672:5672 \
  -p 15672:15672 \
  rabbitmq:4-management
```

Acesse dashboard: http://localhost:15672 (guest:guest)

### 2. Publicar Mensagem de Teste

```bash
# Via amqp-utils (se instalado)
echo '{"file_path":"/tmp/test.xlsx","farm_id":"farm-1","user_id":"user-1"}' | \
  amqp-publish -H localhost -e excel_processing_queue -b

# Ou usar ferramenta como RabbitMQ Admin UI
```

### 3. Observar Consumer

```bash
go run ./cmd/worker/main.go
# Esperado:
# [RabbitMQ] Conexão estabelecida com sucesso
# [RabbitMQ] Consumer ativo | Aguardando mensagens na fila 'excel_processing_queue'...
# [RabbitMQ] Mensagem recebida | FarmID: farm-1 | UserID: user-1 | Arquivo: /tmp/test.xlsx
# [Handler] Processando planilha: /tmp/test.xlsx | Fazenda: farm-1 | Usuário: user-1
# [RabbitMQ] Mensagem processada com sucesso | FarmID: farm-1
```

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
