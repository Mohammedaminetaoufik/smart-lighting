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
		api.POST("/lcus", handleCreateLCUJSON(db))
		api.GET("/lcus/:id", handleGetLCUJSON(db))
		api.POST("/lcus/:id/test", handleTestLCU(db, lcuAdapter))
		api.POST("/lcus/:id/sync", handleSyncLCU(db, lcuAdapter))
		api.GET("/lcus/:id/lampadaires", handleGetLCULampadaires(db))

		// Lampadaires API
		api.GET("/lampadaires/:id", handleGetLampadaireJSON(db))
		api.GET("/lampadaires/missing-location", handleGetMissingLocationLampadaires(db))
		api.POST("/lampadaires/:id/location", handleUpdateLampadaireLocation(db))
		api.POST("/lampadaires/:id/commissioning", handleUpdateCommissioningStatus(db))

		// Lighting Profiles API
		api.GET("/lighting-profiles", handleGetLightingProfiles(db))
		api.POST("/lighting-profiles", handleCreateLightingProfile(db))
		api.POST("/lighting-profiles/:id/apply", handleApplyLightingProfile(db, lcuAdapter))
		api.GET("/lighting-profiles/:id/details", handleGetLightingProfileDetails(db))
		api.POST("/lighting-profiles/:id/enable", handleEnableLightingProfile(db))
		api.POST("/lighting-profiles/:id/disable", handleDisableLightingProfile(db))

		// Lighting Groups API
		api.GET("/lighting-groups", handleGetLightingGroups(db))
		api.POST("/lighting-groups", handleCreateLightingGroup(db))

		// Interventions API
		api.GET("/interventions", handleGetInterventions(db))
		api.POST("/interventions", handleCreateIntervention(db))
		api.POST("/alerts/:id/intervention", handleCreateInterventionFromAlert(db))
		api.POST("/interventions/:id/start", handleUpdateInterventionStatus(db, "in_progress"))
		api.POST("/interventions/:id/resolve", handleUpdateInterventionStatus(db, "resolved"))
		api.POST("/interventions/:id/close", handleCloseIntervention(db))

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
		api.GET("/simulator/scenarios", handleGetScenarios())
		api.POST("/simulator/scenario", handleRunScenario(db))

		// Basestations API
		api.GET("/basestations", handleGetBasestations(db))
		api.POST("/basestations", handleCreateBasestation(db))
		api.GET("/basestations/:id", handleGetBasestation(db))
		api.POST("/basestations/:id/simulate-offline", handleSimulateBasestationOffline(db))
		api.GET("/basestations/:id/controllers", handleGetBasestationControllers(db))

		// Cabinets API
		api.GET("/cabinets", handleGetCabinets(db))
		api.POST("/cabinets", handleCreateCabinet(db))
		api.GET("/cabinets/:id", handleGetCabinet(db))
		api.GET("/cabinets/:id/circuits", handleGetCabinetCircuits(db))
		api.POST("/cabinets/:id/circuits", handleCreateCabinetCircuit(db))
		api.POST("/cabinets/:id/simulate-door-open", handleSimulateCabinetDoorOpen(db))
		api.POST("/cabinets/:id/simulate-power-failure", handleSimulatePowerFailure(db))

		// Controllers API
		api.GET("/controllers", handleGetControllers(db))
		api.POST("/controllers", handleCreateController(db))
		api.GET("/controllers/:id", handleGetController(db))
		api.POST("/controllers/:id/associate", handleAssociateController(db))

		// Work Orders API
		api.GET("/workorders", handleGetWorkOrders(db))
		api.POST("/workorders", handleCreateWorkOrder(db))
		api.GET("/workorders/:id", handleGetWorkOrder(db))
		api.POST("/workorders/from-alerts", handleCreateWorkOrderFromAlerts(db))
		api.POST("/workorders/:id/assign", handleAssignWorkOrder(db))
		api.POST("/workorders/:id/start", handleStartWorkOrder(db))
		api.POST("/workorders/:id/resolve", handleResolveWorkOrder(db))
		api.POST("/workorders/:id/close", handleCloseWorkOrder(db))

		// Alerts extended
		api.POST("/alerts/:id/ack", handleAckAlert(db))
		api.POST("/alerts/:id/close", handleCloseAlert(db))

		// Dashboard extended
		api.GET("/dashboard/network-health", handleGetNetworkHealth(db))
		api.GET("/dashboard/commissioning-progress", handleGetCommissioningProgress(db))

		// Commissioning workflow
		api.POST("/commissioning/:id/advance", handleAdvanceCommissioning(db))
		api.POST("/commissioning/:id/test-comm", handleTestCommCommissioning(db))
		api.POST("/commissioning/:id/test-dimming", handleTestDimmingCommissioning(db))
		api.POST("/commissioning/:id/validate", handleValidateCommissioning(db))
		api.POST("/commissioning/:id/fail", handleFailCommissioning(db))
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
