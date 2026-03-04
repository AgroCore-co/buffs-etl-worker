package domain

// BrincoLookup mapeia brinco (string) → id_bufalo (UUID string).
// Carregado em memória a partir do PostgreSQL uma única vez por mensagem,
// antes de processar as abas dependentes (Pesagens, Sanitário, etc.).
//
// Complexidade de busca: O(1) por linha da planilha.
// Memória: ~100 bytes por animal (brinco de ~10 chars + UUID de 36 chars + overhead do map).
// Para 10.000 animais: ~1MB — cabe tranquilamente em RAM.
type BrincoLookup map[string]string
