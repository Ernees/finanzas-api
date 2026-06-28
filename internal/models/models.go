package models

import "time"

type Transaction struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Amount        float64   `json:"amount"`
	Description   string    `json:"description"`
	CategoryID    *string   `json:"category_id,omitempty"`
	Date          time.Time `json:"date"`
	Source        string    `json:"source"`
	ImportBatchID *string   `json:"import_batch_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type CreateTransactionRequest struct {
	Amount      float64   `json:"amount"`
	Description string    `json:"description"`
	CategoryID  *string   `json:"category_id,omitempty"`
	Date        time.Time `json:"date"`
}

type ListTransactionsResponse struct {
	Items []Transaction `json:"items"`
	Total int           `json:"total"`
}
