// Package domain contém as entidades centrais do domínio do worker.
// Essas structs representam os dados de negócio e não possuem dependência de infraestrutura.
package domain

// ExcelProcessingMessage representa o payload da mensagem publicada pelo NestJS
// no RabbitMQ quando um produtor faz upload de uma planilha Excel.
type ExcelProcessingMessage struct {
	// FilePath é o caminho do arquivo .xlsx salvo em disco (ou S3 futuramente).
	FilePath string `json:"file_path"`

	// FarmID é o identificador único da fazenda associada ao upload.
	FarmID string `json:"farm_id"`

	// UserID é o identificador do usuário que realizou o upload.
	UserID string `json:"user_id"`
}
