// Command api is the backend for the fullstack-app example: a plain Go
// net/http server with an in-memory user store, bcrypt password hashing,
// and bearer-token auth. It is a normal native binary with no wasm build tag,
// separate from the vane wasm frontend that calls it over HTTP.
//
// Run with: go run ./api   (listens on :8081)
package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	store := NewStore()
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/register", registerHandler(store))
	mux.HandleFunc("POST /api/login", loginHandler(store))
	mux.HandleFunc("POST /api/logout", withAuth(store, logoutHandler(store)))
	mux.HandleFunc("GET /api/me", withAuth(store, meHandler(store)))

	mux.HandleFunc("GET /api/users", withAuth(store, listUsersHandler(store)))
	mux.HandleFunc("GET /api/users/{id}", withAuth(store, getUserHandler(store)))
	mux.HandleFunc("PATCH /api/users/{id}", withAuth(store, updateUserHandler(store)))
	mux.HandleFunc("DELETE /api/users/{id}", withAuth(store, deleteUserHandler(store)))

	mux.HandleFunc("GET /api/stats", withAuth(store, statsHandler(store)))

	mux.HandleFunc("GET /api/users/{id}/notes", withAuth(store, listNotesHandler(store)))
	mux.HandleFunc("POST /api/users/{id}/notes", withAuth(store, createNoteHandler(store)))
	mux.HandleFunc("PATCH /api/notes/{id}", withAuth(store, updateNoteHandler(store)))
	mux.HandleFunc("DELETE /api/notes/{id}", withAuth(store, deleteNoteHandler(store)))

	log.Println("api listening on http://localhost:8081")
	srv := &http.Server{
		Addr:              ":8081",
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
