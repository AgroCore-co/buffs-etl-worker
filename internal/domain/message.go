// Package domain contém as entidades centrais do domínio do worker.
// Essas structs representam os dados de negócio e não possuem dependência de infraestrutura.
package domain

// ExcelProcessingMessage representa o payload da mensagem publicada pelo NestJS
// no RabbitMQ quando um produtor faz upload de uma planilha Excel.
//
// Os campos JSON devem corresponder exatamente ao que o NestJS publica:
//
//	{ "file_path", "propriedade_id", "usuario_id", "timestamp" }
type ExcelProcessingMessage struct {
	// FilePath é o caminho relativo do arquivo .xlsx dentro de temp/uploads/.
	// O caminho absoluto é resolvido no worker usando UPLOAD_BASE_PATH.
	FilePath string `json:"file_path"`

	// PropriedadeID é o UUID da propriedade rural associada ao upload.
	PropriedadeID string `json:"propriedade_id"`

	// UsuarioID é o UUID do usuário que realizou o upload.
	UsuarioID string `json:"usuario_id"`

	// Timestamp é a data/hora ISO 8601 em que a mensagem foi criada pelo NestJS.
	Timestamp string `json:"timestamp"`
}
