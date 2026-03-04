// Package main é o ponto de entrada do buffs-etl-worker.
// Este worker consome mensagens do RabbitMQ contendo referências a planilhas Excel
// enviadas por produtores rurais, e processa os dados de forma assíncrona e eficiente.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jaobarreto/buffs-etl-worker/internal/config"
	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
	"github.com/jaobarreto/buffs-etl-worker/internal/excel"
	"github.com/jaobarreto/buffs-etl-worker/internal/infrastructure/messaging"
	"github.com/jaobarreto/buffs-etl-worker/internal/port"
	"github.com/jaobarreto/buffs-etl-worker/internal/repository"
	"github.com/joho/godotenv"
)

func main() {
	// ─── Carrega variáveis de ambiente do arquivo .env (se existir) ──────
	// Em produção/Docker, as vars são injetadas diretamente pelo container.
	// godotenv.Load() falha silenciosamente se .env não existir.
	if err := godotenv.Load(); err != nil {
		log.Println("[INFO] Arquivo .env não encontrado. Usando variáveis de ambiente do sistema ou fallbacks.")
	}

	log.Println("=== BUFFS ETL Worker ===")
	log.Println("Iniciando worker de processamento de planilhas...")

	// ─── Carrega configurações ───────────────────────────────────────────
	cfg := config.Load()

	// ─── Cria o consumer do RabbitMQ ─────────────────────────────────────
	consumer, err := messaging.NewRabbitMQConsumer(cfg.RabbitMQ)
	if err != nil {
		log.Fatalf("Erro fatal ao criar consumer RabbitMQ: %v", err)
	}
	defer consumer.Close()

	// ─── Contexto com cancelamento por sinal do SO ───────────────────────
	// Captura SIGINT (Ctrl+C) e SIGTERM (docker stop) para shutdown gracioso.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("Sinal recebido: %v — iniciando shutdown gracioso...", sig)
		cancel()
	}()

	// ─── Conecta ao PostgreSQL (para lookup de brincos) ─────────────────
	var brincoLoader port.BrincoLoader
	if cfg.DatabaseURL != "" {
		pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("Erro fatal ao conectar ao PostgreSQL: %v", err)
		}
		defer pool.Close()
		brincoLoader = repository.NewPostgresBrincoLoader(pool)
		log.Println("Conexão PostgreSQL estabelecida para lookup de brincos.")
	} else {
		log.Println("[AVISO] DATABASE_URL não configurada. Resolução brinco→UUID desativada.")
	}

	// ─── Cria o processador de planilhas ─────────────────────────────────
	processor := excel.NewProcessor(cfg.UploadBasePath, brincoLoader)
	log.Printf("Upload base path: %s", cfg.UploadBasePath)

	// ─── Handler de mensagens: orquestra o ETL completo ──────────────────
	handler := func(ctx context.Context, msg domain.ExcelProcessingMessage) error {
		log.Printf("[Handler] Recebida mensagem para processar: %s | Propriedade: %s | Usuário: %s",
			msg.FilePath, msg.PropriedadeID, msg.UsuarioID,
		)

		result, err := processor.Process(ctx, msg)
		if err != nil {
			log.Printf("[Handler] ERRO ao processar '%s': %v", msg.FilePath, err)
			return err // NACK — volta para a fila / DLQ
		}

		log.Printf("[Handler] Processamento concluído: %d inseridos, %d ignorados, %d erros",
			result.TotalInserted(), result.TotalSkipped(), len(result.AllErrors()),
		)

		return nil // ACK — mensagem processada com sucesso
	}

	// ─── Inicia o consumer (bloqueia até contexto ser cancelado) ─────────
	log.Println("Consumer iniciado. Aguardando mensagens...")

	if err := consumer.Start(ctx, handler); err != nil {
		if ctx.Err() != nil {
			// Shutdown gracioso — não é um erro real
			log.Println("Worker encerrado com sucesso.")
			return
		}
		log.Fatalf("Erro fatal no consumer: %v", err)
	}

	log.Println("Worker encerrado com sucesso.")
}
