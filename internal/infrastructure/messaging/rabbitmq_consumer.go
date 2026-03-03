// Package messaging implementa os adapters de infraestrutura para
// comunicação com o RabbitMQ. Este pacote é a implementação concreta
// da interface port.MessageConsumer.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jaobarreto/buffs-etl-worker/internal/config"
	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
	"github.com/jaobarreto/buffs-etl-worker/internal/port"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Garante em tempo de compilação que RabbitMQConsumer implementa a interface port.MessageConsumer.
var _ port.MessageConsumer = (*RabbitMQConsumer)(nil)

// RabbitMQConsumer é o adapter que consome mensagens de uma fila do RabbitMQ.
// Possui reconexão automática caso a conexão caia.
type RabbitMQConsumer struct {
	cfg  config.RabbitMQConfig
	conn *amqp.Connection
	ch   *amqp.Channel
	mu   sync.Mutex // protege conn e ch durante reconexões
	done chan struct{}
}

// NewRabbitMQConsumer cria uma nova instância do consumer e estabelece
// a conexão inicial com o RabbitMQ.
func NewRabbitMQConsumer(cfg config.RabbitMQConfig) (*RabbitMQConsumer, error) {
	consumer := &RabbitMQConsumer{
		cfg:  cfg,
		done: make(chan struct{}),
	}

	// Primeira conexão — se falhar, retornamos erro imediatamente
	if err := consumer.connect(); err != nil {
		return nil, fmt.Errorf("falha na conexão inicial com RabbitMQ: %w", err)
	}

	log.Println("[RabbitMQ] Conexão estabelecida com sucesso")
	return consumer, nil
}

// connect estabelece a conexão TCP e abre um canal AMQP.
// Também declara a fila para garantir que ela exista (idempotente).
func (c *RabbitMQConsumer) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := amqp.Dial(c.cfg.URL)
	if err != nil {
		return fmt.Errorf("erro ao conectar no RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("erro ao abrir canal AMQP: %w", err)
	}

	// Declara a fila de forma idempotente (cria se não existir).
	// Durable=true garante que a fila sobrevive a restarts do RabbitMQ.
	_, err = ch.QueueDeclare(
		c.cfg.QueueName, // nome da fila
		true,            // durable — sobrevive restart do broker
		false,           // autoDelete — não deletar quando sem consumidores
		false,           // exclusive — permite múltiplos consumidores
		false,           // noWait — aguardar confirmação do servidor
		nil,             // argumentos extras
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("erro ao declarar fila '%s': %w", c.cfg.QueueName, err)
	}

	// Configura prefetch para processar uma mensagem por vez.
	// Isso evita que o worker pegue mais mensagens do que consegue processar.
	if err := ch.Qos(1, 0, false); err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("erro ao configurar QoS: %w", err)
	}

	c.conn = conn
	c.ch = ch

	log.Printf("[RabbitMQ] Canal aberto | Fila: %s | Prefetch: 1", c.cfg.QueueName)
	return nil
}

// reconnect tenta reconectar ao RabbitMQ em loop até conseguir ou o contexto ser cancelado.
func (c *RabbitMQConsumer) reconnect(ctx context.Context) error {
	delay := time.Duration(c.cfg.ReconnectDelaySec) * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Printf("[RabbitMQ] Tentando reconectar em %v...", delay)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}

		if err := c.connect(); err != nil {
			log.Printf("[RabbitMQ] Falha na reconexão: %v", err)
			continue
		}

		log.Println("[RabbitMQ] Reconexão realizada com sucesso!")
		return nil
	}
}

// Start inicia o loop de consumo de mensagens da fila.
// Implementa a interface port.MessageConsumer.
// Este método bloqueia até que o contexto seja cancelado.
func (c *RabbitMQConsumer) Start(ctx context.Context, handler port.MessageHandler) error {
	for {
		// Verifica se o contexto já foi cancelado antes de iniciar/reiniciar
		select {
		case <-ctx.Done():
			log.Println("[RabbitMQ] Contexto cancelado, encerrando consumer...")
			return ctx.Err()
		default:
		}

		err := c.consume(ctx, handler)
		if err == nil {
			// Encerrou normalmente (contexto cancelado)
			return nil
		}

		log.Printf("[RabbitMQ] Consumer desconectado: %v", err)

		// Tenta reconectar antes de reiniciar o consumo
		if reconnErr := c.reconnect(ctx); reconnErr != nil {
			return fmt.Errorf("falha permanente na reconexão: %w", reconnErr)
		}
	}
}

// consume registra o consumer na fila e processa mensagens em loop.
// Retorna erro quando a conexão cai ou o contexto é cancelado.
func (c *RabbitMQConsumer) consume(ctx context.Context, handler port.MessageHandler) error {
	c.mu.Lock()
	ch := c.ch
	c.mu.Unlock()

	// Registra o consumer na fila
	deliveries, err := ch.ConsumeWithContext(
		ctx,
		c.cfg.QueueName, // fila
		"",              // consumer tag (vazio = gerado automaticamente)
		false,           // autoAck — desativado para controle manual de ACK/NACK
		false,           // exclusive
		false,           // noLocal
		false,           // noWait
		nil,             // args
	)
	if err != nil {
		return fmt.Errorf("erro ao registrar consumer: %w", err)
	}

	log.Printf("[RabbitMQ] Consumer ativo | Aguardando mensagens na fila '%s'...", c.cfg.QueueName)

	// Monitora a conexão para detectar quedas
	connCloseCh := c.conn.NotifyClose(make(chan *amqp.Error, 1))

	for {
		select {
		case <-ctx.Done():
			log.Println("[RabbitMQ] Contexto cancelado, parando consumo...")
			return nil

		case amqpErr, ok := <-connCloseCh:
			if !ok {
				return fmt.Errorf("canal de notificação de conexão fechado")
			}
			return fmt.Errorf("conexão com RabbitMQ perdida: %v", amqpErr)

		case delivery, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("canal de deliveries fechado (conexão perdida)")
			}

			c.processDelivery(ctx, delivery, handler)
		}
	}
}

// processDelivery desserializa o payload JSON e delega o processamento ao handler.
// Faz ACK em caso de sucesso ou NACK com requeue em caso de erro.
func (c *RabbitMQConsumer) processDelivery(ctx context.Context, delivery amqp.Delivery, handler port.MessageHandler) {
	var msg domain.ExcelProcessingMessage

	// Desserializa o JSON do payload
	if err := json.Unmarshal(delivery.Body, &msg); err != nil {
		log.Printf("[RabbitMQ] Erro ao desserializar mensagem (descartando): %v | Body: %s", err, string(delivery.Body))
		// Mensagem malformada — faz ACK para não reprocessar lixo infinitamente
		_ = delivery.Ack(false)
		return
	}

	log.Printf("[RabbitMQ] Mensagem recebida | FarmID: %s | UserID: %s | Arquivo: %s",
		msg.FarmID, msg.UserID, msg.FilePath)

	// Delega ao handler (que futuramente executará a lógica de ETL)
	if err := handler(ctx, msg); err != nil {
		log.Printf("[RabbitMQ] Erro ao processar mensagem (reenfileirando): %v", err)
		// NACK com requeue=true — a mensagem volta para a fila para nova tentativa
		_ = delivery.Nack(false, true)
		return
	}

	// Processamento bem-sucedido — confirma a mensagem
	if err := delivery.Ack(false); err != nil {
		log.Printf("[RabbitMQ] Erro ao enviar ACK: %v", err)
	}

	log.Printf("[RabbitMQ] Mensagem processada com sucesso | FarmID: %s", msg.FarmID)
}

// Close encerra a conexão com o RabbitMQ de forma graciosa.
// Implementa a interface port.MessageConsumer.
func (c *RabbitMQConsumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error

	if c.ch != nil {
		if err := c.ch.Close(); err != nil {
			errs = append(errs, fmt.Errorf("erro ao fechar canal: %w", err))
		}
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("erro ao fechar conexão: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("erros ao fechar RabbitMQ: %v", errs)
	}

	log.Println("[RabbitMQ] Conexão encerrada com sucesso")
	return nil
}
