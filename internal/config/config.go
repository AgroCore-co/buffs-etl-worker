// Package config centraliza todas as configurações do worker,
// lendo variáveis de ambiente com fallback para valores padrão de desenvolvimento.
//
// O arquivo .env é carregado automaticamente na inicialização do worker (cmd/worker/main.go)
// usando github.com/joho/godotenv. Em produção/Docker, as variáveis são injetadas
// diretamente pelo container e o .env não é necessário.
package config

import "os"

// Config agrupa todas as configurações necessárias para o worker.
type Config struct {
	RabbitMQ       RabbitMQConfig
	UploadBasePath string
	DatabaseURL    string
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
		// Diretório base onde os uploads são salvos pelo NestJS.
		// Em desenvolvimento local: caminho absoluto para buffs-api/temp/uploads
		// Em Docker: /shared/uploads (bind mount compartilhado)
		UploadBasePath: getEnv("UPLOAD_BASE_PATH", "../buffs-api/temp/uploads"),

		// Connection string PostgreSQL para lookup de brincos.
		// Se vazio, a resolução brinco→UUID fica desativada.
		DatabaseURL: getEnv("DATABASE_URL", ""),
	}
}

// getEnv retorna o valor da variável de ambiente ou o fallback fornecido.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
