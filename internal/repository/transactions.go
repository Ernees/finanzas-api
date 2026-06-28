package repository

import (
	"context"
	"fmt"

	"github.com/Erneees/finanzas-api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TransactionRepository struct {
	db *pgxpool.Pool
}

func NewTransactionRepository(db *pgxpool.Pool) *TransactionRepository {
	return &TransactionRepository{db: db}
}

func (r *TransactionRepository) Insert(ctx context.Context, userID string, req models.CreateTransactionRequest) (*models.Transaction, error) {
	const q = `
		INSERT INTO transactions (user_id, amount, description, category_id, date, source)
		VALUES ($1, $2, $3, $4, $5, 'manual')
		RETURNING id, user_id, amount, description, category_id, date, source, import_batch_id, created_at`

	tx := &models.Transaction{}
	err := r.db.QueryRow(ctx, q,
		userID,
		req.Amount,
		req.Description,
		req.CategoryID,
		req.Date,
	).Scan(
		&tx.ID,
		&tx.UserID,
		&tx.Amount,
		&tx.Description,
		&tx.CategoryID,
		&tx.Date,
		&tx.Source,
		&tx.ImportBatchID,
		&tx.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert transaction: %w", err)
	}
	return tx, nil
}

func (r *TransactionRepository) List(ctx context.Context, userID string, month string) ([]models.Transaction, error) {
	if month != "" {
		const q = `
			SELECT id, user_id, amount, description, category_id, date, source, import_batch_id, created_at
			FROM transactions
			WHERE user_id = $1
			  AND to_char(date, 'YYYY-MM') = $2
			ORDER BY date DESC`
		rows, err := r.db.Query(ctx, q, userID, month)
		if err != nil {
			return nil, fmt.Errorf("list transactions: %w", err)
		}
		defer rows.Close()
		return scanTransactions(rows)
	}

	const q = `
		SELECT id, user_id, amount, description, category_id, date, source, import_batch_id, created_at
		FROM transactions
		WHERE user_id = $1
		ORDER BY date DESC`
	rows, err := r.db.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()
	return scanTransactions(rows)
}

func (r *TransactionRepository) Delete(ctx context.Context, userID string, transactionID string) error {
	const q = `DELETE FROM transactions WHERE id = $1 AND user_id = $2`
	tag, err := r.db.Exec(ctx, q, transactionID, userID)
	if err != nil {
		return fmt.Errorf("delete transaction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type pgxRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanTransactions(rows pgxRows) ([]models.Transaction, error) {
	var txs []models.Transaction
	for rows.Next() {
		var tx models.Transaction
		if err := rows.Scan(
			&tx.ID,
			&tx.UserID,
			&tx.Amount,
			&tx.Description,
			&tx.CategoryID,
			&tx.Date,
			&tx.Source,
			&tx.ImportBatchID,
			&tx.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		txs = append(txs, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return txs, nil
}
