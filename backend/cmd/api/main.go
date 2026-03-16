package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/time/rate"
)

type Config struct {
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	ServerPort     string
	RateLimitRPS   float64
	RateLimitBurst int
}

type App struct {
	db      *sql.DB
	mux     *http.ServeMux
	limiter *rate.Limiter
}

func loadConfig() Config {
	return Config{
		DBHost:         getEnv("DB_HOST", "postgres"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "appuser"),
		DBPassword:     getEnv("DB_PASSWORD", "changeme"),
		DBName:         getEnv("DB_NAME", "appdb"),
		ServerPort:     getEnv("SERVER_PORT", "8080"),
		RateLimitRPS:   10.0,
		RateLimitBurst: 20,
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func (app *App) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.limiter.Allow() {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *App) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "https://localhost")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *App) healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := app.db.PingContext(ctx); err != nil {
		http.Error(w, "Database unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	stats := app.db.Stats()
	w.Write([]byte(fmt.Sprintf(`{"status":"healthy","database":"ok","connections":{"active":%d,"idle":%d,"in_use":%d,"max":%d}}`,
		stats.OpenConnections, stats.Idle, stats.InUse, stats.MaxOpenConnections)))
}

func (app *App) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ready":true}`))
}

func (app *App) apiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Write([]byte(`{"message":"API v1","endpoints":["/health","/ready","/db/time"]}`))
}

func (app *App) dbTimeHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var dbTime string
	err := app.db.QueryRowContext(ctx, "SELECT NOW()::text").Scan(&dbTime)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"db_time":"%s"}`, dbTime)))
}

func (app *App) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	stats := app.db.Stats()
	output := fmt.Sprintf(`# HELP postgres_connections PostgreSQL connection stats
# TYPE postgres_connections gauge
postgres_connections_active %d
postgres_connections_idle %d
postgres_connections_in_use %d
postgres_connections_wait_count %d
postgres_connections_max_connections %d
`, stats.OpenConnections, stats.Idle, stats.InUse, stats.WaitCount, stats.MaxOpenConnections)
	w.Write([]byte(output))
}

func (app *App) routes() {
	app.mux = http.NewServeMux()
	app.mux.HandleFunc("/health", app.healthHandler)
	app.mux.HandleFunc("/ready", app.readyHandler)
	app.mux.HandleFunc("/api/v1", app.apiHandler)
	app.mux.HandleFunc("/db/time", app.dbTimeHandler)
	app.mux.HandleFunc("/metrics", app.metricsHandler)
}

func (app *App) connectDB(cfg Config) error {
	// For local development: sslmode=disable
	// For production (cloud DB): sslmode=require or sslmode=verify-full
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)

	var err error
	app.db, err = sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("unable to connect to database: %w", err)
	}

	app.db.SetMaxOpenConns(25)
	app.db.SetMaxIdleConns(5)
	app.db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.db.PingContext(ctx); err != nil {
		return fmt.Errorf("unable to ping database: %w", err)
	}

	log.Println("Connected to PostgreSQL database")
	return nil
}

func main() {
	cfg := loadConfig()

	app := &App{
		limiter: rate.NewLimiter(rate.Limit(cfg.RateLimitRPS), cfg.RateLimitBurst),
	}

	if err := app.connectDB(cfg); err != nil {
		log.Printf("Database connection error: %v", err)
	}

	app.routes()

	handler := app.corsMiddleware(app.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			http.ServeFile(w, r, "./static/index.html")
			return
		}
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/api/v1" ||
			r.URL.Path == "/db/time" || r.URL.Path == "/metrics" {
			app.mux.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, "./static/"+r.URL.Path)
	})))

	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Starting server on port %s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	if app.db != nil {
		app.db.Close()
	}

	log.Println("Server exited")
}
