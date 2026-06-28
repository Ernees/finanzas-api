package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey int

const userIDKey contextKey = iota

func GetUserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok
}

func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeUnauthorized(w)
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		secret := []byte(os.Getenv("SUPABASE_JWT_SECRET"))

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return secret, nil
		})
		if err != nil || !token.Valid {
			writeUnauthorized(w)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			writeUnauthorized(w)
			return
		}

		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			writeUnauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, sub)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
