package main

import (
	"net/http"
	"strconv"
	"strings"
)

func listNotesHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if _, ok := s.UserByID(userID); !ok {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, s.NotesByUser(userID))
	}
}

type noteRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// createNoteHandler only allows a user to create notes for themselves.
func createNoteHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if userID != userIDFromContext(r) {
			writeError(w, http.StatusForbidden, "you can only add notes to your own account")
			return
		}
		var req noteRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			writeError(w, http.StatusBadRequest, "title is required")
			return
		}
		writeJSON(w, http.StatusCreated, s.CreateNote(userID, req.Title, req.Body))
	}
}

// updateNoteHandler and deleteNoteHandler check ownership via the note's own
// UserID rather than a path user id. The route is /api/notes/{id}, scoped to
// the note itself.
func updateNoteHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid note id")
			return
		}
		note, ok := s.NoteByID(id)
		if !ok {
			writeError(w, http.StatusNotFound, "note not found")
			return
		}
		if note.UserID != userIDFromContext(r) {
			writeError(w, http.StatusForbidden, "you can only edit your own notes")
			return
		}
		var req noteRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			writeError(w, http.StatusBadRequest, "title is required")
			return
		}
		updated, _ := s.UpdateNote(id, req.Title, req.Body)
		writeJSON(w, http.StatusOK, updated)
	}
}

func deleteNoteHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid note id")
			return
		}
		note, ok := s.NoteByID(id)
		if !ok {
			writeError(w, http.StatusNotFound, "note not found")
			return
		}
		if note.UserID != userIDFromContext(r) {
			writeError(w, http.StatusForbidden, "you can only delete your own notes")
			return
		}
		s.DeleteNote(id)
		w.WriteHeader(http.StatusNoContent)
	}
}
