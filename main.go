package main

import (
	"context"
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	db, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := ensureSchema(db); err != nil {
		log.Fatal(err)
	}

	// Background service: mark offline lampadaires every minute
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			markInactiveLampadairesOffline(context.Background(), db)
		}
	}()

	lcuAdapter := NewLCUAdapter()

	router := gin.Default()
	router.Static("/static", "./static")
	router.SetHTMLTemplate(template.Must(template.ParseFiles("templates/index.tmpl")))

	// Page routes
	router.GET("/", handleIndex(db))
	router.GET("/lcus", handleListLCUs(db))
	router.POST("/lcus", handleCreateLCU(db))
	router.POST("/lcus/:id", handleUpdateLCU(db))
	router.POST("/lampadaires", handleCreateLampadaire(db))
	router.POST("/lampadaires/:id", handleUpdateLampadaire(db))
	router.POST("/lampadaires/:id/archive", handleArchiveLampadaire(db))
	router.POST("/lampadaires/:id/restore", handleRestoreLampadaire(db))

	// JSON API
	api := router.Group("/api")
	{
		// Users & Logs
		api.GET("/users", handleGetUsers(db))
		api.POST("/users", handleCreateUser(db))
		api.GET("/logs", handleGetLogs(db))

		// LCU API
		api.GET("/lcus", handleListLCUsJSON(db))
		api.GET("/lcus/:id", handleGetLCUJSON(db))
		api.POST("/lcus/:id/test", handleTestLCU(db, lcuAdapter))
		api.POST("/lcus/:id/sync", handleSyncLCU(db, lcuAdapter))
		api.GET("/lcus/:id/lampadaires", handleGetLCULampadaires(db))

		// Lampadaires API
		api.GET("/lampadaires/:id", handleGetLampadaireJSON(db))
		api.GET("/lampadaires/missing-location", handleListMissingLocation(db))
		api.POST("/lampadaires/:id/location", handleUpdateLampadaireLocationAPI(db))

		// Telemetry API
		api.POST("/telemetry", handlePostTelemetry(db))
		api.GET("/lampadaires/:id/telemetry", handleGetTelemetry(db))
		api.GET("/lampadaires/:id/telemetry/latest", handleGetTelemetryLatest(db))

		// Dimming API
		api.POST("/lampadaires/:id/dimming", handlePostDimming(db, lcuAdapter))
		api.GET("/lampadaires/:id/dimming", handleGetDimmingHistory(db))

		// Alerts API
		api.GET("/alerts", handleGetAlerts(db))
		api.GET("/alerts/counts", handleGetAlertCounts(db))
		api.GET("/alerts/summary", handleGetAlertSummary(db))
		api.POST("/alerts/:id/resolve", handleResolveAlert(db))

		// Calculator API
		api.POST("/calculateur/run/:id", handleRunCalculator(db, lcuAdapter))
		api.POST("/calculateur/run-all", handleRunCalculatorAll(db, lcuAdapter))
		api.GET("/lampadaires/:id/decisions", handleGetDecisions(db))

		// Dashboard API
		api.GET("/dashboard/stats", handleGetDashboardStats(db))

		// Energy API
		api.GET("/energy/summary", handleGetEnergySummary(db))

		// Simulator API
		api.POST("/simulator/telemetry/:id", handleSimulateTelemetry(db))
		api.POST("/simulator/telemetry/all", handleSimulateAll(db))
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		log.Printf("Serveur démarré sur http://localhost:%s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Arrêt du serveur en cours...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Arrêt forcé du serveur : ", err)
	}

	log.Println("Serveur arrêté proprement.")
}

func handleGetEnergySummary(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		summary, err := getEnergySummary(c.Request.Context(), db)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors du calcul énergétique.")
			return
		}
		respondJSON(c, http.StatusOK, summary)
	}
}
