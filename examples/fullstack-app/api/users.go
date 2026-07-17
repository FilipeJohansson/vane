package main

import (
	"net/http"
	"strconv"
	"strings"
)

func listUsersHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.ListUsers())
	}
}

func getUserHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		user, ok := s.UserByID(id)
		if !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, user)
	}
}

type updateUserRequest struct {
	Name string `json:"name"`
}

// updateUserHandler only allows a user to edit their own name. The path id must
// match the authenticated token's user id.
func updateUserHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if id != userIDFromContext(r) {
			writeError(w, http.StatusForbidden, "you can only edit your own profile")
			return
		}
		var req updateUserRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		user, ok := s.UpdateName(id, req.Name)
		if !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, user)
	}
}

// deleteUserHandler only allows a user to delete their own account.
func deleteUserHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if id != userIDFromContext(r) {
			writeError(w, http.StatusForbidden, "you can only delete your own account")
			return
		}
		if !s.DeleteUser(id) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type statsResponse struct {
	TotalUsers  int    `json:"totalUsers"`
	MemberSince string `json:"memberSince"`
}

func statsHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.UserByID(userIDFromContext(r))
		if !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, statsResponse{
			TotalUsers:  s.Count(),
			MemberSince: user.CreatedAt.Format("2006-01-02"),
		})
	}
}
