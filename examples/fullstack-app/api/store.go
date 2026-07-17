package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

// User is the backend's record for a registered account.
// PasswordHash is tagged json:"-" so it never leaves the process in a response body.
type User struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

var ErrEmailTaken = errors.New("email already registered")

// Note is a piece of text owned by a user, a minimal one-to-many relation
// standing in for whatever "user has many X" a real app would model.
type Note struct {
	ID        int       `json:"id"`
	UserID    int       `json:"userId"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// Store is an in-memory repository standing in for a real database.
// Every method locks around the shared maps so concurrent requests are safe.
type Store struct {
	mu         sync.RWMutex
	users      map[int]*User
	byEmail    map[string]int
	tokens     map[string]int // token -> userID
	notes      map[int]*Note
	nextID     int
	nextNoteID int
}

func NewStore() *Store {
	return &Store{
		users:      make(map[int]*User),
		byEmail:    make(map[string]int),
		tokens:     make(map[string]int),
		notes:      make(map[int]*Note),
		nextID:     1,
		nextNoteID: 1,
	}
}

func (s *Store) CreateUser(name, email, passwordHash string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.byEmail[email]; ok {
		return nil, ErrEmailTaken
	}

	u := &User{
		ID:           s.nextID,
		Name:         name,
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC(),
	}
	s.users[u.ID] = u
	s.byEmail[email] = u.ID
	s.nextID++
	return u, nil
}

func (s *Store) UserByEmail(email string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byEmail[email]
	if !ok {
		return nil, false
	}
	return s.users[id], true
}

func (s *Store) UserByID(id int) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	return u, ok
}

func (s *Store) ListUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users)
}

func (s *Store) UpdateName(id int, name string) (*User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return nil, false
	}
	u.Name = name
	return u, true
}

func (s *Store) DeleteUser(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return false
	}
	delete(s.users, id)
	delete(s.byEmail, u.Email)
	for tok, uid := range s.tokens {
		if uid == id {
			delete(s.tokens, tok)
		}
	}
	for noteID, n := range s.notes {
		if n.UserID == id {
			delete(s.notes, noteID)
		}
	}
	return true
}

// NewToken mints an opaque bearer token for userID. A real backend would use
// signed/expiring JWTs; a random token in a server-side map is enough here.
func (s *Store) NewToken(userID int) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing means the OS entropy source is broken
	}
	token := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = userID
	return token
}

func (s *Store) UserIDForToken(token string) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.tokens[token]
	return id, ok
}

func (s *Store) RevokeToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
}

func (s *Store) CreateNote(userID int, title, body string) *Note {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := &Note{
		ID:        s.nextNoteID,
		UserID:    userID,
		Title:     title,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}
	s.notes[n.ID] = n
	s.nextNoteID++
	return n
}

func (s *Store) NoteByID(id int) (*Note, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.notes[id]
	return n, ok
}

// NotesByUser returns userID's notes, most recent first.
func (s *Store) NotesByUser(userID int) []*Note {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Note, 0)
	for _, n := range s.notes {
		if n.UserID == userID {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Store) UpdateNote(id int, title, body string) (*Note, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notes[id]
	if !ok {
		return nil, false
	}
	n.Title = title
	n.Body = body
	return n, true
}

func (s *Store) DeleteNote(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.notes[id]; !ok {
		return false
	}
	delete(s.notes, id)
	return true
}
