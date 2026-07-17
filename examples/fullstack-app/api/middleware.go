package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// frontendOrigin is the dev server address vane run serves the wasm frontend from.
const frontendOrigin = "http://localhost:8080"

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", frontendOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ctxKey int

const userIDKey ctxKey = 0

func withUserID(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// userIDFromContext returns the authenticated user id set by withAuth.
// Only call from handlers wrapped in withAuth, since the id is always present there.
func userIDFromContext(r *http.Request) int {
	return r.Context().Value(userIDKey).(int)
}

// withAuth requires a valid "Authorization: Bearer <token>" header, resolves it
// to a user id via the store, and passes it to next through the request context.
func withAuth(s *Store, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}
		userID, ok := s.UserIDForToken(token)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		r = r.WithContext(withUserID(r.Context(), userID))
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
