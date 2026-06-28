package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Erneees/finanzas-api/internal/middleware"
	"github.com/Erneees/finanzas-api/internal/models"
	"github.com/Erneees/finanzas-api/internal/repository"
	"github.com/go-chi/chi/v5"
)

type TransactionSvc interface {
	CreateTransaction(ctx context.Context, userID string, req models.CreateTransactionRequest) (*models.Transaction, error)
	ListTransactions(ctx context.Context, userID string, month string) ([]models.Transaction, error)
	DeleteTransaction(ctx context.Context, userID string, transactionID string) error
}

type TransactionHandler struct {
	svc TransactionSvc
}

func NewTransactionHandler(svc TransactionSvc) *TransactionHandler {
	return &TransactionHandler{svc: svc}
}

func (h *TransactionHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	month := r.URL.Query().Get("month")
	txs, err := h.svc.ListTransactions(r.Context(), userID, month)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if txs == nil {
		txs = []models.Transaction{}
	}

	writeJSON(w, http.StatusOK, models.ListTransactionsResponse{
		Items: txs,
		Total: len(txs),
	})
}

func (h *TransactionHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req models.CreateTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	tx, err := h.svc.CreateTransaction(r.Context(), userID, req)
	if err != nil {
		if isValidationError(err) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, tx)
}

func (h *TransactionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	id := chi.URLParam(r, "id")
	err := h.svc.DeleteTransaction(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "transaction not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func isValidationError(err error) bool {
	msg := err.Error()
	return msg == "amount must be greater than zero" || msg == "description is required"
}
