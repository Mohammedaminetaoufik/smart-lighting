package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"map-interactif/internal/controllers"
	"map-interactif/internal/middleware"
	"map-interactif/internal/repository"
	"map-interactif/internal/services"
)

func main() {
	_ = godotenv.Load()

	db, err := repository.OpenDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := repository.EnsureSchema(db); err != nil {
		log.Fatal(err)
	}

	if err := repository.SeedMockDataIfEmpty(db); err != nil {
		log.Printf("Warning: Database seeder failed: %v", err)
	}

	// Charge le profil de dimming appris du dataset (data-driven). Vide = le
	// calculateur retombe sur ses règles ; rempli par tools/import-telemetry.
	if err := services.LoadDimmingReference(context.Background(), db); err != nil {
		log.Printf("Warning: chargement dimming_reference: %v", err)
	} else if services.DimmingReferenceReady() {
		log.Println("Profil de dimming data-driven chargé.")
	}

	// Seuils de détection de panne (maintenance prédictive), configurables.
	if err := services.LoadFaultThresholds(context.Background(), db); err != nil {
		log.Printf("Warning: chargement fault_thresholds: %v", err)
	}

	// Background service: mark offline lampadaires every minute
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			if err := repository.MarkInactiveLampadairesOffline(context.Background(), db); err != nil {
				services.Heartbeat(context.Background(), db, "mark_offline", "error", err.Error())
			} else {
				services.Heartbeat(context.Background(), db, "mark_offline", "ok", "")
			}
		}
	}()

	// Background service: daily data retention (telemetry + audit logs)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		runRetention := func() {
			ctx := context.Background()
			// Get retention setting (default 90 days)
			retention := 90
			db.QueryRowContext(ctx,
				"SELECT COALESCE(value::int, 90) FROM system_settings WHERE key='job.telemetry_retention_days'").Scan(&retention)

			if _, err := db.ExecContext(ctx,
				"DELETE FROM sensor_measurements WHERE created_at < NOW() - ($1::text || ' days')::interval",
				retention); err != nil {
				services.Heartbeat(ctx, db, "data_retention", "error", err.Error())
				return
			}
			// Audit logs older than 1 year
			db.ExecContext(ctx, "DELETE FROM access_logs WHERE created_at < NOW() - INTERVAL '1 year'")
			// AI query logs older than 90 days (table created by the AI service —
			// may not exist on first run, error ignored on purpose)
			db.ExecContext(ctx, "DELETE FROM ai_query_logs WHERE created_at < NOW() - INTERVAL '90 days'")
			services.Heartbeat(ctx, db, "data_retention", "ok", "")
		}
		// Run once at startup then daily
		go runRetention()
		for range ticker.C {
			runRetention()
		}
	}()

	lcuAdapter := services.NewLCUAdapter()

	router := gin.Default()

	// NOTE: L'ancienne interface HTML server-rendered (routes de pages "/",
	// "/lcus" + formulaires POST) a été supprimée. Elle dupliquait le frontend
	// React et exposait des routes de mutation SANS authentification. Toutes les
	// opérations passent désormais par l'API JSON /api/* protégée par JWT.

	// Auth routes (public — no JWT required)
	// Rate limit login : 5 tentatives/minute par IP contre le brute-force.
	loginLimiter := middleware.NewRateLimiter(5, time.Minute)
	authGroup := router.Group("/api/auth")
	{
		authGroup.POST("/login", loginLimiter.Middleware(), controllers.HandleLogin(db))
		// Protected auth routes
		authGroup.GET("/me", middleware.JWTMiddleware(), controllers.HandleMe())
		authGroup.POST("/change-password", middleware.JWTMiddleware(), controllers.HandleChangePassword(db))
		authGroup.POST("/admin/reset-password", middleware.JWTMiddleware(), middleware.RequireRole("admin"), controllers.HandleAdminResetPassword(db))
	}

	// JSON API — all routes require authentication
	api := router.Group("/api", middleware.JWTMiddleware())
	{
		// Users & Logs — write operations restricted to admin
		api.GET("/users", controllers.HandleGetUsers(db))
		api.POST("/users", middleware.RequireRole("admin"), controllers.HandleCreateUser(db))
		api.GET("/users/:id", controllers.HandleGetUser(db))
		api.PATCH("/users/:id", middleware.RequireRole("admin"), controllers.HandleUpdateUser(db))
		api.DELETE("/users/:id", middleware.RequireRole("admin"), controllers.HandleDeleteUser(db))
		api.GET("/logs", controllers.HandleGetLogs(db))

		// LCU API
		api.GET("/lcus", controllers.HandleListLCUsJSON(db))
		api.POST("/lcus", controllers.HandleCreateLCUJSON(db))
		api.GET("/lcus/:id", controllers.HandleGetLCUJSON(db))
		api.PUT("/lcus/:id", controllers.HandleUpdateLCUJSON(db))
		api.POST("/lcus/:id/test", controllers.HandleTestLCU(db, lcuAdapter))
		api.POST("/lcus/:id/sync", controllers.HandleSyncLCU(db, lcuAdapter))
		api.GET("/lcus/:id/lampadaires", controllers.HandleGetLCULampadaires(db))
		api.POST("/lcus/:id/bulk-dim", controllers.HandleBulkDimLCU(db))

		// Lampadaires API
		api.GET("/lampadaires", controllers.HandleListLampadairesJSON(db))
		api.GET("/lampadaires/missing-location", controllers.HandleGetMissingLocationLampadaires(db))
		api.GET("/lampadaires/:id", controllers.HandleGetLampadaireJSON(db))
		api.PATCH("/lampadaires/:id", controllers.HandlePatchLampadaireJSON(db))
		api.POST("/lampadaires/:id/location", controllers.HandleUpdateLampadaireLocation(db))
		api.POST("/lampadaires/:id/commissioning", controllers.HandleUpdateCommissioningStatus(db))
		api.POST("/lampadaires/:id/assign-lcu", controllers.HandleAssignLCU(db))
		api.GET("/lampadaires/:id/diagnostic", controllers.HandleDiagnoseLampadaire(db))

		// Lighting Profiles API
		api.GET("/lighting-profiles", controllers.HandleGetLightingProfiles(db))
		api.POST("/lighting-profiles", controllers.HandleCreateLightingProfile(db))
		api.POST("/lighting-profiles/:id/apply", controllers.HandleApplyLightingProfile(db, lcuAdapter))
		api.GET("/lighting-profiles/:id/details", controllers.HandleGetLightingProfileDetails(db))
		api.POST("/lighting-profiles/:id/enable", controllers.HandleEnableLightingProfile(db))
		api.POST("/lighting-profiles/:id/disable", controllers.HandleDisableLightingProfile(db))
		api.PUT("/lighting-profiles/:id", controllers.HandleUpdateLightingProfile(db))
		api.DELETE("/lighting-profiles/:id", middleware.RequireRole("admin"), controllers.HandleDeleteLightingProfile(db))

		// Lighting Groups API
		api.GET("/lighting-groups", controllers.HandleGetLightingGroups(db))
		api.POST("/lighting-groups", controllers.HandleCreateLightingGroup(db))

		// Interventions API
		api.GET("/interventions", controllers.HandleGetInterventions(db))
		api.POST("/interventions", controllers.HandleCreateIntervention(db))
		api.POST("/alerts/:id/intervention", controllers.HandleCreateInterventionFromAlert(db))
		api.POST("/interventions/:id/start", controllers.HandleUpdateInterventionStatus(db, "in_progress"))
		api.POST("/interventions/:id/resolve", controllers.HandleUpdateInterventionStatus(db, "resolved"))
		api.POST("/interventions/:id/close", controllers.HandleCloseIntervention(db))

		// Telemetry API
		api.POST("/telemetry", controllers.HandlePostTelemetry(db))
		api.GET("/lampadaires/:id/telemetry", controllers.HandleGetTelemetry(db))
		api.GET("/lampadaires/:id/telemetry/latest", controllers.HandleGetTelemetryLatest(db))

		// Dimming API
		api.POST("/lampadaires/:id/dimming", controllers.HandlePostDimming(db, lcuAdapter))
		api.GET("/lampadaires/:id/dimming", controllers.HandleGetDimmingHistory(db))

		// Alerts API
		api.GET("/alerts", controllers.HandleGetAlerts(db))
		api.GET("/alerts/counts", controllers.HandleGetAlertCounts(db))
		api.GET("/alerts/summary", controllers.HandleGetAlertSummary(db))
		api.GET("/alerts/timeline", controllers.HandleGetAlertTimeline(db))
		api.POST("/alerts/:id/resolve", controllers.HandleResolveAlert(db))

		// Calculator API
		api.POST("/calculateur/run/:id", controllers.HandleRunCalculator(db, lcuAdapter))
		api.POST("/calculateur/run-all", controllers.HandleRunCalculatorAll(db, lcuAdapter))
		api.GET("/lampadaires/:id/decisions", controllers.HandleGetDecisions(db))
		api.GET("/calculateur/evaluate-dataset", middleware.RequireRole("admin"), controllers.HandleEvaluateDataset(db))

		// Dashboard API
		api.GET("/dashboard/stats", controllers.HandleGetDashboardStats(db))

		// Energy API
		api.GET("/energy/summary", controllers.HandleGetEnergySummary(db))
		api.GET("/energy/daily", controllers.HandleGetDailyEnergy(db))
		api.GET("/energy/top-consumers", controllers.HandleGetEnergyTopConsumers(db))
		api.GET("/energy/anomalies", controllers.HandleGetEnergyAnomalies(db))
		api.GET("/energy/hourly", controllers.HandleGetEnergyHourly(db))
		api.GET("/energy/recommendations", controllers.HandleGetEnergyRecommendations(db))

		// Calendrier astronomique (lever/coucher soleil — allumage crépusculaire)
		api.GET("/astronomy/sun", controllers.HandleGetSunTimes())

		// Maintenance prédictive — lampadaires à risque, stats et historique de pannes
		api.GET("/faults/at-risk", controllers.HandleGetAtRiskLamps(db))
		api.GET("/faults/stats", controllers.HandleGetFaultStats(db))
		api.GET("/lampadaires/:id/faults", controllers.HandleGetLampFaults(db))
		// Endpoints prédictifs enrichis (score, confiance, échéance, signaux, tendance)
		api.GET("/faults/predictions", controllers.HandleGetPredictions(db))
		api.GET("/faults/predictive-summary", controllers.HandleGetPredictiveSummary(db))
		api.GET("/faults/trend", controllers.HandleGetRiskTrend(db))
		api.GET("/lampadaires/:id/prediction", controllers.HandleGetLampPrediction(db))

		// Financier — tarification ONEE, facture réelle, synthèse direction
		api.GET("/energy/tariffs", controllers.HandleGetTariffs(db))
		api.PUT("/energy/tariffs", middleware.RequireRole("admin"), controllers.HandleUpdateTariffs(db))
		api.GET("/energy/bill", controllers.HandleGetEnergyBill(db))
		api.GET("/finance/summary", controllers.HandleGetFinancialSummary(db))

		// Simulator API
		api.POST("/simulator/telemetry/:id", controllers.HandleSimulateTelemetry(db))
		api.POST("/simulator/telemetry/all", controllers.HandleSimulateAll(db))
		api.GET("/simulator/scenarios", controllers.HandleGetScenarios())
		api.POST("/simulator/scenario", controllers.HandleRunScenario(db))

		// Basestations API
		api.GET("/basestations", controllers.HandleGetBasestations(db))
		api.POST("/basestations", controllers.HandleCreateBasestation(db))
		api.GET("/basestations/:id", controllers.HandleGetBasestation(db))
		api.POST("/basestations/:id/simulate-offline", controllers.HandleSimulateBasestationOffline(db))
		api.GET("/basestations/:id/controllers", controllers.HandleGetBasestationControllers(db))

		// Cabinets API
		api.GET("/cabinets", controllers.HandleGetCabinets(db))
		api.POST("/cabinets", controllers.HandleCreateCabinet(db))
		api.GET("/cabinets/:id", controllers.HandleGetCabinet(db))
		api.GET("/cabinets/:id/circuits", controllers.HandleGetCabinetCircuits(db))
		api.POST("/cabinets/:id/circuits", controllers.HandleCreateCabinetCircuit(db))
		api.POST("/cabinets/:id/simulate-door-open", controllers.HandleSimulateCabinetDoorOpen(db))
		api.POST("/cabinets/:id/simulate-power-failure", controllers.HandleSimulatePowerFailure(db))

		// Controllers API
		api.GET("/controllers", controllers.HandleGetControllers(db))
		api.POST("/controllers", controllers.HandleCreateController(db))
		api.GET("/controllers/:id", controllers.HandleGetController(db))
		api.POST("/controllers/:id/associate", controllers.HandleAssociateController(db))

		// Work Orders API
		api.GET("/workorders", controllers.HandleGetWorkOrders(db))
		api.GET("/workorders/open", controllers.HandleGetOpenWorkOrders(db))
		api.POST("/workorders", controllers.HandleCreateWorkOrder(db))
		api.GET("/workorders/:id", controllers.HandleGetWorkOrder(db))
		api.POST("/workorders/from-alerts", controllers.HandleCreateWorkOrderFromAlerts(db))
		api.POST("/workorders/:id/assign", controllers.HandleAssignWorkOrder(db))
		api.POST("/workorders/:id/accept", controllers.HandleAcceptWorkOrder(db))
		api.POST("/workorders/:id/start", controllers.HandleStartWorkOrder(db))
		api.POST("/workorders/:id/resolve", controllers.HandleResolveWorkOrder(db))
		api.POST("/workorders/:id/close", controllers.HandleCloseWorkOrder(db))
		api.POST("/workorders/:id/cancel", controllers.HandleCancelWorkOrder(db))
		api.POST("/workorders/:id/reopen", controllers.HandleReopenWorkOrder(db))
		api.POST("/workorders/:id/add-note", controllers.HandleAddWorkOrderNote(db))
		api.GET("/workorders/:id/alerts", controllers.HandleGetWorkOrderAlerts(db))
		api.GET("/workorders/:id/interventions", controllers.HandleGetWorkOrderInterventions(db))
		api.POST("/workorders/:id/interventions", controllers.HandleCreateWorkOrderIntervention(db))
		api.GET("/workorders/:id/logs", controllers.HandleGetWorkOrderLogs(db))

		// Alerts extended
		api.POST("/alerts/:id/ack", controllers.HandleAckAlert(db))
		api.POST("/alerts/:id/close", controllers.HandleCloseAlert(db))
		api.POST("/alerts/:id/create-work-order", controllers.HandleCreateWorkOrderFromAlert(db))

		// Dashboard extended
		api.GET("/dashboard/network-health", controllers.HandleGetNetworkHealth(db))
		api.GET("/dashboard/commissioning-progress", controllers.HandleGetCommissioningProgress(db))

		// Commissioning workflow
		api.POST("/commissioning/:id/advance", controllers.HandleAdvanceCommissioning(db))
		api.POST("/commissioning/:id/test-comm", controllers.HandleTestCommCommissioning(db))
		api.POST("/commissioning/:id/test-dimming", controllers.HandleTestDimmingCommissioning(db))
		api.POST("/commissioning/:id/validate", controllers.HandleValidateCommissioning(db))
		api.POST("/commissioning/:id/fail", controllers.HandleFailCommissioning(db))
		api.POST("/commissioning/batch-test", controllers.HandleBatchTest(db))
		api.POST("/commissioning/validate-successful", controllers.HandleValidateSuccessful(db))
		api.POST("/commissioning/retry-failed", controllers.HandleRetryFailed(db))

		// Bulk operations
		api.PATCH("/lampadaires/bulk", controllers.HandleBulkUpdateLampadaires(db))
		api.POST("/lampadaires/bulk/archive", controllers.HandleBulkArchiveLampadaires(db))
		api.POST("/alerts/bulk-action", controllers.HandleBulkAlertAction(db))
		api.POST("/workorders/bulk-assign", controllers.HandleBulkAssignWorkOrders(db))

		// CSV Exports
		api.GET("/export/lampadaires", controllers.HandleExportLampadaires(db))
		api.GET("/export/alerts", controllers.HandleExportAlerts(db))
		api.GET("/export/workorders", controllers.HandleExportWorkOrders(db))
		api.GET("/export/energy", controllers.HandleExportEnergy(db))

		// Audit Log
		api.GET("/audit-logs", controllers.HandleGetAuditLogs(db))
		api.GET("/audit-logs/summary", controllers.HandleGetAuditSummary(db))
		api.GET("/audit-logs/:id", controllers.HandleGetAuditLog(db))

		// Global search
		api.GET("/search", controllers.HandleGlobalSearch(db))

		// AI Service proxy
		api.GET("/ai/health", controllers.HandleAIHealth())
		api.POST("/ai/query", controllers.HandleAIQuery())
		api.POST("/ai/query/stream", controllers.HandleAIQueryStream())
		api.POST("/ai/feedback", controllers.HandleAIFeedback())
		api.GET("/ai/history", controllers.HandleAIHistory())
		api.GET("/ai/page-insights/:page", controllers.HandleAIPageInsights())
		api.GET("/ai/suggestions", controllers.HandleAISuggestions())
		api.GET("/ai/daily-digest", controllers.HandleAIDailyDigest())
		api.GET("/ai/entity-insights/:entityType/:entityId", controllers.HandleAIEntityInsights())
		api.GET("/ai/decision-center", controllers.HandleAIDecisionCenter())

		// System / observability
		api.GET("/health", controllers.HandleHealth(db))
		api.GET("/system/health", controllers.HandleSystemHealth(db))
		api.GET("/system/version", controllers.HandleSystemVersion)
		api.GET("/system/jobs", controllers.HandleSystemJobs(db))
		api.GET("/system/config", controllers.HandleGetSystemConfig(db))
		api.PUT("/system/config", middleware.RequireRole("admin"), controllers.HandleUpdateSystemConfig(db))

		// Maintenance Windows
		api.GET("/maintenance-windows", controllers.HandleGetMaintenanceWindows(db))
		api.GET("/maintenance-windows/active", controllers.HandleGetActiveMaintenanceWindows(db))
		api.GET("/maintenance-windows/upcoming", controllers.HandleGetUpcomingMaintenanceWindows(db))
		api.GET("/maintenance-windows/check", controllers.HandleCheckMaintenance(db))
		api.POST("/maintenance-windows", middleware.RequireRole("admin"), controllers.HandleCreateMaintenanceWindow(db))
		api.GET("/maintenance-windows/:id", controllers.HandleGetMaintenanceWindow(db))
		api.PUT("/maintenance-windows/:id", middleware.RequireRole("admin"), controllers.HandleUpdateMaintenanceWindow(db))
		api.POST("/maintenance-windows/:id/cancel", middleware.RequireRole("admin"), controllers.HandleCancelMaintenanceWindow(db))
		api.POST("/maintenance-windows/:id/complete", middleware.RequireRole("admin"), controllers.HandleCompleteMaintenanceWindow(db))
		api.DELETE("/maintenance-windows/:id", middleware.RequireRole("admin"), controllers.HandleDeleteMaintenanceWindow(db))
		api.GET("/maintenance-windows/:id/workorders", controllers.HandleGetMaintenanceWindowWorkOrders(db))

		// CSV import — opération de masse réservée à l'admin
		api.POST("/lampadaires/import", middleware.RequireRole("admin"), controllers.HandleImportLampadaires(db))
	}

	// Mobile API — technician app routes (JWT required)
	mobile := router.Group("/api/mobile", middleware.JWTMiddleware())
	{
		// AI diagnostic (proxied to FastAPI)
		mobile.GET("/ai/missions", controllers.HandleAIMobileMissions())
		mobile.GET("/ai/lampadaires/:id/diagnostic", controllers.HandleAIMobileDiagnosticLampadaire())
		mobile.GET("/ai/lcus/:id/diagnostic", controllers.HandleAIMobileDiagnosticLCU())
		mobile.GET("/ai/workorders/:id/diagnostic", controllers.HandleAIMobileDiagnosticWorkOrder())
		// WorkOrder photos (handlers existed but were not wired)
		mobile.POST("/workorders/:id/photos", controllers.HandleUploadWorkOrderPhoto(db))
		mobile.GET("/workorders/:id/photos", controllers.HandleListWorkOrderPhotos(db))
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
