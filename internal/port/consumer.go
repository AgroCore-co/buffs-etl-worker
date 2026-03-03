// Package port define as interfaces (portas) que a camada de domínio expõe
// para a infraestrutura implementar. Seguindo o padrão Ports & Adapters (Hexagonal Architecture).
package port

import (
	"context"

	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
)

// MessageHandler é a função de callback que será chamada para cada mensagem
// consumida da fila. A lógica de processamento (parse Excel, bulk insert, etc.)
// será injetada através dessa função.
type MessageHandler func(ctx context.Context, msg domain.ExcelProcessingMessage) error

// MessageConsumer define o contrato para qualquer implementação de consumidor
// de mensagens (RabbitMQ, Kafka, SQS, etc.). Isso desacopla o domínio da
// tecnologia de mensageria utilizada.
type MessageConsumer interface {
	// Start inicia o consumo da fila e processa mensagens usando o handler fornecido.
	// Bloqueia até que o contexto seja cancelado ou ocorra um erro irrecuperável.
	Start(ctx context.Context, handler MessageHandler) error

	// Close encerra a conexão com o broker de forma graciosa.
	Close() error
}
