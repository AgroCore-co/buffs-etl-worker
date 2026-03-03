// Package config centraliza todas as configurações do worker,
// lendo variáveis de ambiente com fallback para valores padrão de desenvolvimento.
package config

import "os"

// Config agrupa todas as configurações necessárias para o worker.
type Config struct {
	RabbitMQ RabbitMQConfig
}

// RabbitMQConfig contém os parâmetros de conexão com o RabbitMQ.
type RabbitMQConfig struct {
	// URL é a connection string AMQP (ex: amqp://guest:guest@localhost:5672/).
	URL string

	// QueueName é o nome da fila que o consumer vai escutar.
	QueueName string

	// ReconnectDelaySec é o intervalo em segundos entre tentativas de reconexão.
	ReconnectDelaySec int
}

// Load carrega as configurações a partir de variáveis de ambiente.
// Utiliza valores padrão para facilitar o desenvolvimento local.
func Load() *Config {
	return &Config{
		RabbitMQ: RabbitMQConfig{
			URL:               getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
			QueueName:         getEnv("RABBITMQ_QUEUE", "excel_processing_queue"),
			ReconnectDelaySec: 5,
		},
	}
}

// getEnv retorna o valor da variável de ambiente ou o fallback fornecido.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
