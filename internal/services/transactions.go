package services

import (
	"context"
	"errors"

	"github.com/Erneees/finanzas-api/internal/models"
	"github.com/Erneees/finanzas-api/internal/repository"
)

type TransactionRepo interface {
	Insert(ctx context.Context, userID string, req models.CreateTransactionRequest) (*models.Transaction, error)
	List(ctx context.Context, userID string, month string) ([]models.Transaction, error)
	Delete(ctx context.Context, userID string, transactionID string) error
}

type TransactionService struct {
	repo TransactionRepo
}

func NewTransactionService(repo TransactionRepo) *TransactionService {
	return &TransactionService{repo: repo}
}

func (s *TransactionService) CreateTransaction(ctx context.Context, userID string, req models.CreateTransactionRequest) (*models.Transaction, error) {
	if req.Amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}
	if req.Description == "" {
		return nil, errors.New("description is required")
	}
	return s.repo.Insert(ctx, userID, req)
}

func (s *TransactionService) ListTransactions(ctx context.Context, userID string, month string) ([]models.Transaction, error) {
	return s.repo.List(ctx, userID, month)
}

func (s *TransactionService) DeleteTransaction(ctx context.Context, userID string, transactionID string) error {
	err := s.repo.Delete(ctx, userID, transactionID)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.ErrNotFound
	}
	return err
}
