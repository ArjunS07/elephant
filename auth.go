package main

import (
	"context"
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

type loginResponse struct {
    Token string `json:"token"`
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
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

    var userID string
    err = dbpool.QueryRow(
        context.Background(),
        "INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id",
        req.Username, string(hashed),
    ).Scan(&userID)
    if err != nil {
        http.Error(w, "Username may already exist", http.StatusBadRequest)
        return
    }

    // Create JWT token
    _, tokenString, _ := tokenAuth.Encode(map[string]interface{}{"user_id": userID})

    resp := loginResponse{Token: tokenString}
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
    var req loginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }

    var userID string
    var passwordHash string
    err := dbpool.QueryRow(context.Background(),
        "SELECT id, password_hash FROM users WHERE username = $1", req.Username).Scan(&userID, &passwordHash)
    if err != nil {
        http.Error(w, "Invalid username or password", http.StatusUnauthorized)
        return
    }

    if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
        http.Error(w, "Invalid username or password", http.StatusUnauthorized)
        return
    }

    // Create JWT token
    _, tokenString, _ := tokenAuth.Encode(map[string]interface{}{"user_id": userID})

    resp := loginResponse{Token: tokenString}
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}
