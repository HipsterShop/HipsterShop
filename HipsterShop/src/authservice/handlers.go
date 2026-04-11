package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

var jwtSigningMethod = jwt.SigningMethodHS256
var tokenExpiry = 24 * time.Hour

type handlers struct {
	users     *mongo.Collection
	events    *mongo.Collection
	jwtSecret []byte
	log       *logrus.Logger
}

func newHandlers(db *mongo.Database, jwtSecret string, log *logrus.Logger) *handlers {
	coll := db.Collection("users")
	events := db.Collection("auth_events")
	// Unique index on email
	idxModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	coll.Indexes().CreateOne(context.Background(), idxModel)
	events.Indexes().CreateOne(context.Background(), mongo.IndexModel{Keys: bson.D{{Key: "createdAt", Value: 1}}})
	events.Indexes().CreateOne(context.Background(), mongo.IndexModel{Keys: bson.D{{Key: "eventType", Value: 1}}})
	return &handlers{users: coll, events: events, jwtSecret: []byte(jwtSecret), log: log}
}

// --- Models ---

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"`
	Email        string             `bson:"email"`
	Name         string             `bson:"name"`
	PasswordHash string             `bson:"passwordHash"`
	CreatedAt    time.Time          `bson:"createdAt"`
}

type Claims struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	jwt.RegisteredClaims
}

// --- Handlers ---

// POST /signup  { "name": "...", "email": "...", "password": "..." }
func (h *handlers) signup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		jsonError(w, "email and password are required", http.StatusBadRequest)
		return
	}
	if len(req.Password) < 8 {
		jsonError(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	// Hash password with bcrypt cost 12
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		h.log.Errorf("bcrypt error: %v", err)
		jsonError(w, "Internal error", http.StatusInternalServerError)
		return
	}

	user := User{
		ID:           primitive.NewObjectID(),
		Email:        strings.ToLower(strings.TrimSpace(req.Email)),
		Name:         req.Name,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if _, err := h.users.InsertOne(ctx, user); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			jsonError(w, "Email already registered", http.StatusConflict)
		} else {
			h.log.Errorf("MongoDB insert error: %v", err)
			jsonError(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	token, err := h.issueToken(user)
	if err != nil {
		jsonError(w, "Failed to issue token", http.StatusInternalServerError)
		return
	}

	h.setTokenCookie(w, token)
	h.recordAuthEvent(r.Context(), "signup_success", user.ID.Hex(), user.Email, "")
	jsonOK(w, map[string]string{"userId": user.ID.Hex(), "email": user.Email, "name": user.Name})
}

// POST /login  { "email": "...", "password": "..." }
func (h *handlers) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var user User
	err := h.users.FindOne(ctx, bson.M{"email": strings.ToLower(strings.TrimSpace(req.Email))}).Decode(&user)
	if err != nil {
		// Return same error for wrong email or wrong password (prevent user enumeration)
		h.recordAuthEvent(r.Context(), "login_failed", "", strings.ToLower(strings.TrimSpace(req.Email)), "user_not_found")
		jsonError(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		h.recordAuthEvent(r.Context(), "login_failed", user.ID.Hex(), user.Email, "bad_password")
		jsonError(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	token, err := h.issueToken(user)
	if err != nil {
		jsonError(w, "Failed to issue token", http.StatusInternalServerError)
		return
	}

	h.setTokenCookie(w, token)
	h.recordAuthEvent(r.Context(), "login_success", user.ID.Hex(), user.Email, "")
	jsonOK(w, map[string]string{"userId": user.ID.Hex(), "email": user.Email, "name": user.Name})
}

// POST /logout
func (h *handlers) logout(w http.ResponseWriter, r *http.Request) {
	claims, _ := h.extractClaims(r)
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	if claims != nil {
		h.recordAuthEvent(r.Context(), "logout", claims.UserID, claims.Email, "")
	} else {
		h.recordAuthEvent(r.Context(), "logout", "", "", "anonymous")
	}
	jsonOK(w, map[string]string{"message": "logged out"})
}

// GET /me — returns user info from JWT cookie
func (h *handlers) me(w http.ResponseWriter, r *http.Request) {
	claims, err := h.extractClaims(r)
	if err != nil {
		jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	jsonOK(w, map[string]string{"userId": claims.UserID, "email": claims.Email, "name": claims.Name})
}

// --- Helpers ---

func (h *handlers) issueToken(user User) (string, error) {
	claims := &Claims{
		UserID: user.ID.Hex(),
		Email:  user.Email,
		Name:   user.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "hipstershop-authservice",
		},
	}
	return jwt.NewWithClaims(jwtSigningMethod, claims).SignedString(h.jwtSecret)
}

func (h *handlers) setTokenCookie(w http.ResponseWriter, token string) {
	secure := os.Getenv("USE_HTTPS") == "1"
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		MaxAge:   int(tokenExpiry.Seconds()),
		HttpOnly: true,    // NOT accessible via JS → XSS safe
		Secure:   secure,  // Set true when HTTPS is configured
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *handlers) extractClaims(r *http.Request) (*Claims, error) {
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		return nil, err
	}
	claims := &Claims{}
	_, err = jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
		return h.jwtSecret, nil
	})
	return claims, err
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (h *handlers) recordAuthEvent(ctx context.Context, eventType, userID, email, reason string) {
	if h.events == nil {
		return
	}
	entry := bson.M{
		"eventType": eventType,
		"userId":    userID,
		"email":     email,
		"reason":    reason,
		"createdAt": time.Now().UTC(),
	}
	writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := h.events.InsertOne(writeCtx, entry); err != nil {
		h.log.WithError(err).Debug("failed to record auth event")
	}
}
