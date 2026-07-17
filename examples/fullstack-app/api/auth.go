package main

import (
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

func registerHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))

		if req.Name == "" || !strings.Contains(req.Email, "@") {
			writeError(w, http.StatusBadRequest, "name and a valid email are required")
			return
		}
		if len(req.Password) < 6 {
			writeError(w, http.StatusBadRequest, "password must be at least 6 characters")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not hash password")
			return
		}

		user, err := s.CreateUser(req.Name, req.Email, string(hash))
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, authResponse{Token: s.NewToken(user.ID), User: user})
	}
}

func loginHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))

		user, ok := s.UserByEmail(req.Email)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}

		writeJSON(w, http.StatusOK, authResponse{Token: s.NewToken(user.ID), User: user})
	}
}

func meHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.UserByID(userIDFromContext(r))
		if !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, user)
	}
}

func logoutHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		s.RevokeToken(token)
		w.WriteHeader(http.StatusNoContent)
	}
}
