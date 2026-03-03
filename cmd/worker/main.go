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

	"github.com/jaobarreto/buffs-etl-worker/internal/config"
	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
	"github.com/jaobarreto/buffs-etl-worker/internal/infrastructure/messaging"
)

func main() {
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

	// ─── Handler temporário (stub) ───────────────────────────────────────
	// TODO: Substituir por um Use Case real que faz parse da planilha com
	// Excelize e bulk insert no PostgreSQL via Goroutines.
	handler := func(ctx context.Context, msg domain.ExcelProcessingMessage) error {
		log.Printf("[Handler] Processando planilha: %s | Fazenda: %s | Usuário: %s",
			msg.FilePath, msg.FarmID, msg.UserID,
		)

		// Aqui entrará a lógica de ETL:
		// 1. Ler arquivo Excel com excelize
		// 2. Parsear linhas em structs do domínio
		// 3. Bulk insert no PostgreSQL via goroutines
		// 4. Notificar conclusão (opcional)

		return nil
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
