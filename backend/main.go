package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
	DBMaxOpenConns int
	DBMaxIdleConns int
}

type App struct {
	db      *sql.DB
	mux     *http.ServeMux
	limiter *rate.Limiter
}

type Bin struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Request struct {
	ID        int               `json:"id"`
	BinID     string            `json:"bin_id"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body,omitempty"`
	ClientIP  string            `json:"client_ip"`
	CreatedAt time.Time         `json:"created_at"`
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
		DBMaxOpenConns: getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns: getEnvInt("DB_MAX_IDLE_CONNS", 5),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func generateID(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
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
	w.Write([]byte(`{"message":"API v1","endpoints":["POST /api/bins","GET /api/bins","GET /api/bins/:id","GET /api/bins/:id/requests","DELETE /api/bins/:id","GET /:bin_id"]}`))
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

func (app *App) createBinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	id := generateID(8)
	bin := Bin{
		ID:        id,
		Name:      req.Name,
		CreatedAt: time.Now(),
	}

	_, err := app.db.ExecContext(r.Context(),
		"INSERT INTO bins (id, name, created_at) VALUES ($1, $2, $3)",
		bin.ID, bin.Name, bin.CreatedAt)
	if err != nil {
		log.Printf("Error creating bin: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bin)
}

func (app *App) listBinsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := app.db.QueryContext(r.Context(),
		"SELECT id, name, created_at FROM bins ORDER BY created_at DESC LIMIT 100")
	if err != nil {
		log.Printf("Error listing bins: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var bins []Bin
	for rows.Next() {
		var bin Bin
		if err := rows.Scan(&bin.ID, &bin.Name, &bin.CreatedAt); err != nil {
			continue
		}
		bins = append(bins, bin)
	}

	if bins == nil {
		bins = []Bin{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bins)
}

func (app *App) getBinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/bins/")
	if id == "" || id == "api/bins" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	var bin Bin
	err := app.db.QueryRowContext(r.Context(),
		"SELECT id, name, created_at FROM bins WHERE id = $1", id).
		Scan(&bin.ID, &bin.Name, &bin.CreatedAt)
	if err == sql.ErrNoRows {
		http.Error(w, "Bin not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Error getting bin: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bin)
}

func (app *App) captureRequestHandler(w http.ResponseWriter, r *http.Request) {
	binID := strings.TrimPrefix(r.URL.Path, "/")
	if binID == "" {
		http.Error(w, "Bin ID required", http.StatusBadRequest)
		return
	}

	// Check bin exists asynchronously - fire and forget
	go func() {
		var exists bool
		_ = app.db.QueryRowContext(context.Background(),
			"SELECT EXISTS(SELECT 1 FROM bins WHERE id = $1)", binID).Scan(&exists)
	}()

	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.RemoteAddr
		if idx := strings.LastIndex(clientIP, ":"); idx != -1 {
			clientIP = clientIP[:idx]
		}
	}

	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	body, _ := io.ReadAll(r.Body)
	headersJSON, _ := json.Marshal(headers)

	// Async write - spawn goroutine
	go func() {
		_, err := app.db.ExecContext(context.Background(),
			`INSERT INTO requests (bin_id, method, path, headers, body, client_ip, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			binID, r.Method, r.URL.Path, headersJSON, string(body), clientIP, time.Now())
		if err != nil {
			log.Printf("Error capturing request: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"status":"captured","bin_id":"%s"}`, binID)
}

func (app *App) getRequestsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/bins/")
	id = strings.TrimSuffix(id, "/requests")
	if id == "" {
		http.Error(w, "Bin ID required", http.StatusBadRequest)
		return
	}

	rows, err := app.db.QueryContext(r.Context(),
		`SELECT id, bin_id, method, path, headers, body, client_ip, created_at 
		 FROM requests WHERE bin_id = $1 ORDER BY created_at DESC LIMIT 100`, id)
	if err != nil {
		log.Printf("Error getting requests: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var requests []Request
	for rows.Next() {
		var req Request
		var headersJSON []byte
		if err := rows.Scan(&req.ID, &req.BinID, &req.Method, &req.Path,
			&headersJSON, &req.Body, &req.ClientIP, &req.CreatedAt); err != nil {
			continue
		}
		json.Unmarshal(headersJSON, &req.Headers)
		requests = append(requests, req)
	}

	if requests == nil {
		requests = []Request{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(requests)
}

func (app *App) deleteBinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/bins/")
	if id == "" {
		http.Error(w, "Bin ID required", http.StatusBadRequest)
		return
	}

	result, err := app.db.ExecContext(r.Context(), "DELETE FROM bins WHERE id = $1", id)
	if err != nil {
		log.Printf("Error deleting bin: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Bin not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *App) routes() {
	app.mux = http.NewServeMux()
	app.mux.HandleFunc("/health", app.healthHandler)
	app.mux.HandleFunc("/ready", app.readyHandler)
	app.mux.HandleFunc("/api/v1", app.apiHandler)
	app.mux.HandleFunc("/db/time", app.dbTimeHandler)
	app.mux.HandleFunc("/metrics", app.metricsHandler)
	app.mux.HandleFunc("/api/bins", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			app.createBinHandler(w, r)
		} else if r.Method == http.MethodGet {
			app.listBinsHandler(w, r)
		}
	})
	app.mux.HandleFunc("/api/bins/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/requests") {
			if r.Method == http.MethodGet {
				app.getRequestsHandler(w, r)
			}
			return
		}
		if r.Method == http.MethodGet {
			app.getBinHandler(w, r)
		} else if r.Method == http.MethodDelete {
			app.deleteBinHandler(w, r)
		}
	})
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

	app.db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	app.db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	app.db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.db.PingContext(ctx); err != nil {
		return fmt.Errorf("unable to ping database: %w", err)
	}

	if err := app.migrate(); err != nil {
		log.Printf("Migration warning: %v", err)
	}

	log.Println("Connected to PostgreSQL database")
	return nil
}

func (app *App) migrate() error {
	ctx := context.Background()

	_, err := app.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS bins (
			id TEXT PRIMARY KEY,
			name TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create bins table: %w", err)
	}

	_, err = app.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS requests (
			id SERIAL PRIMARY KEY,
			bin_id TEXT REFERENCES bins(id) ON DELETE CASCADE,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			headers JSONB DEFAULT '{}',
			body TEXT,
			client_ip TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create requests table: %w", err)
	}

	_, err = app.db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_requests_bin_id ON requests(bin_id)
	`)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	_, err = app.db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_requests_created_at ON requests(created_at)
	`)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	log.Println("Database migration complete")
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

	// Commented out for stress testing
	// handler := app.corsMiddleware(app.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	handler := app.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			http.ServeFile(w, r, "./static/index.html")
			return
		}
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/api/v1" ||
			r.URL.Path == "/db/time" || r.URL.Path == "/metrics" || r.URL.Path == "/api/bins" ||
			strings.HasPrefix(r.URL.Path, "/api/bins/") {
			app.mux.ServeHTTP(w, r)
			return
		}
		// Request Bin capture endpoint
		binID := strings.TrimPrefix(r.URL.Path, "/")
		if binID != "" && !strings.Contains(binID, "/") {
			app.captureRequestHandler(w, r)
			return
		}
		http.ServeFile(w, r, "./static/"+r.URL.Path)
	}))

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
