// Package auth handles user registration, login, and reading the authenticated
// user id out of a request (set by the jwtauth middleware).
package auth

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/jwtauth/v5"
	"golang.org/x/crypto/bcrypt"

	"elephant/internal/store"
)

type Handler struct {
	store     *store.Store
	tokenAuth *jwtauth.JWTAuth
}

func New(st *store.Store, tokenAuth *jwtauth.JWTAuth) *Handler {
	return &Handler{store: st, tokenAuth: tokenAuth}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password required", http.StatusBadRequest)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Error processing password", http.StatusInternalServerError)
		return
	}

	userID, err := h.store.CreateUser(req.Username, string(hashed))
	if err != nil {
		http.Error(w, "Username may already exist", http.StatusBadRequest)
		return
	}

	_, tokenString, _ := h.tokenAuth.Encode(map[string]interface{}{"user_id": userID})

	resp := loginResponse{Token: tokenString}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	userID, passwordHash, err := h.store.UserByUsername(req.Username)
	if err != nil {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	_, tokenString, _ := h.tokenAuth.Encode(map[string]interface{}{"user_id": userID})

	resp := loginResponse{Token: tokenString}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// UserID extracts the authenticated user id from the request context. Returns
// "" if absent.
func UserID(r *http.Request) string {
	_, claims, _ := jwtauth.FromContext(r.Context())
	userID := claims["user_id"]
	if userID == nil {
		return ""
	}
	return userID.(string)
}
