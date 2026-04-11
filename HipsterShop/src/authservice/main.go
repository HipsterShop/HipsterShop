package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const port = "8081"

var log = logrus.New()

func main() {
	log.SetFormatter(&logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	})
	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.InfoLevel)

	mongoURI := mustEnv("MONGO_URI")
	jwtSecret := mustEnv("JWT_SECRET")
	dbName := envOrDefault("MONGO_DATABASE", "auth_db")

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("MongoDB ping failed: %v", err)
	}
	log.Info("Connected to MongoDB")

	db := client.Database(dbName)
	h := newHandlers(db, jwtSecret, log)

	r := mux.NewRouter()

	// Health check
	r.HandleFunc("/_healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	}).Methods(http.MethodGet)

	// Auth endpoints
	r.HandleFunc("/signup", h.signup).Methods(http.MethodPost)
	r.HandleFunc("/login", h.login).Methods(http.MethodPost)
	r.HandleFunc("/logout", h.logout).Methods(http.MethodPost)
	r.HandleFunc("/me", h.me).Methods(http.MethodGet)

	srvPort := port
	if p := os.Getenv("PORT"); p != "" {
		srvPort = p
	}

	log.Infof("authservice listening on :%s", srvPort)
	log.Fatal(http.ListenAndServe(":"+srvPort, r))
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("Required environment variable %q is not set", key)
	}
	return v
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
