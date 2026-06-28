package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Erneees/finanzas-api/internal/handlers"
	"github.com/Erneees/finanzas-api/internal/middleware"
	"github.com/Erneees/finanzas-api/internal/repository"
	"github.com/Erneees/finanzas-api/internal/services"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	if os.Getenv("ENVIRONMENT") != "production" {
		_ = godotenv.Load()
	}

	db, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer db.Close()

	if err := db.Ping(context.Background()); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}

	txRepo := repository.NewTransactionRepository(db)
	txSvc := services.NewTransactionService(txRepo)
	txHandler := handlers.NewTransactionHandler(txSvc)

	r := chi.NewRouter()
	r.Use(middleware.CORS)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	r.Get("/health", handlers.Health)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Get("/transactions", txHandler.List)
		r.Post("/transactions", txHandler.Create)
		r.Delete("/transactions/{id}", txHandler.Delete)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}
