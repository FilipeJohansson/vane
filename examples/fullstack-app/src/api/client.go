// Package api is a thin typed client for the backend in ../../api.
// It uses plain net/http. Under GOOS=js it runs on the browser's Fetch API,
// same as examples/async-fetch, so no wasm-specific code lives here.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const baseURL = "http://localhost:8081"

// User mirrors the backend's public user JSON (password hash never leaves the server).
type User struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt string `json:"createdAt"` // RFC3339, see JoinedDate for display
}

// JoinedDate renders CreatedAt as a short date for display. Falls back to the
// raw value if it doesn't parse, so a backend format change never blanks the UI.
func (u User) JoinedDate() string {
	t, err := time.Parse(time.RFC3339, u.CreatedAt)
	if err != nil {
		return u.CreatedAt
	}
	return t.Format("2006-01-02")
}

type Stats struct {
	TotalUsers  int    `json:"totalUsers"`
	MemberSince string `json:"memberSince"`
}

// Note mirrors the backend's note JSON, a piece of text owned by a user.
type Note struct {
	ID        int    `json:"id"`
	UserID    int    `json:"userId"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

func (n Note) CreatedDate() string {
	t, err := time.Parse(time.RFC3339, n.CreatedAt)
	if err != nil {
		return n.CreatedAt
	}
	return t.Format("2006-01-02")
}

type authResult struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// Error wraps a non-2xx API response, carrying the backend's own message.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

func Register(name, email, password string) (User, string, error) {
	var res authResult
	err := request("POST", "/api/register", "", map[string]string{
		"name": name, "email": email, "password": password,
	}, &res)
	return res.User, res.Token, err
}

func Login(email, password string) (User, string, error) {
	var res authResult
	err := request("POST", "/api/login", "", map[string]string{
		"email": email, "password": password,
	}, &res)
	return res.User, res.Token, err
}

func Logout(token string) error {
	return request("POST", "/api/logout", token, nil, nil)
}

func Me(token string) (User, error) {
	var u User
	err := request("GET", "/api/me", token, nil, &u)
	return u, err
}

func ListUsers(token string) ([]User, error) {
	var users []User
	err := request("GET", "/api/users", token, nil, &users)
	return users, err
}

func GetUser(token string, id int) (User, error) {
	var u User
	err := request("GET", fmt.Sprintf("/api/users/%d", id), token, nil, &u)
	return u, err
}

func UpdateUser(token string, id int, name string) (User, error) {
	var u User
	err := request("PATCH", fmt.Sprintf("/api/users/%d", id), token, map[string]string{"name": name}, &u)
	return u, err
}

func DeleteUser(token string, id int) error {
	return request("DELETE", fmt.Sprintf("/api/users/%d", id), token, nil, nil)
}

func GetStats(token string) (Stats, error) {
	var s Stats
	err := request("GET", "/api/stats", token, nil, &s)
	return s, err
}

func ListNotes(token string, userID int) ([]Note, error) {
	var notes []Note
	err := request("GET", fmt.Sprintf("/api/users/%d/notes", userID), token, nil, &notes)
	return notes, err
}

func CreateNote(token string, userID int, title, body string) (Note, error) {
	var n Note
	err := request("POST", fmt.Sprintf("/api/users/%d/notes", userID), token, map[string]string{
		"title": title, "body": body,
	}, &n)
	return n, err
}

func UpdateNote(token string, id int, title, body string) (Note, error) {
	var n Note
	err := request("PATCH", fmt.Sprintf("/api/notes/%d", id), token, map[string]string{
		"title": title, "body": body,
	}, &n)
	return n, err
}

func DeleteNote(token string, id int) error {
	return request("DELETE", fmt.Sprintf("/api/notes/%d", id), token, nil, nil)
}

func request(method, path, token string, body any, out any) error {
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &Error{Message: "network error: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		msg := errBody.Error
		if msg == "" {
			msg = fmt.Sprintf("request failed with status %d", resp.StatusCode)
		}
		return &Error{Status: resp.StatusCode, Message: msg}
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
