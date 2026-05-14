package main

import (
	"context"
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

	"map-interactif/internal/controllers"
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

	// Background service: mark offline lampadaires every minute
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			repository.MarkInactiveLampadairesOffline(context.Background(), db)
		}
	}()

	lcuAdapter := services.NewLCUAdapter()

	router := gin.Default()
	router.Static("/static", "./static")
	router.SetHTMLTemplate(template.Must(template.ParseFiles("templates/index.tmpl")))

	// Page routes
	router.GET("/", controllers.HandleIndex(db))
	router.GET("/lcus", controllers.HandleListLCUs(db))
	router.POST("/lcus", controllers.HandleCreateLCU(db))
	router.POST("/lcus/:id", controllers.HandleUpdateLCU(db))
	router.POST("/lampadaires", controllers.HandleCreateLampadaire(db))
	router.POST("/lampadaires/:id", controllers.HandleUpdateLampadaire(db))
	router.POST("/lampadaires/:id/archive", controllers.HandleArchiveLampadaire(db))
	router.POST("/lampadaires/:id/restore", controllers.HandleRestoreLampadaire(db))

	// JSON API
	api := router.Group("/api")
	{
		// Users & Logs
		api.GET("/users", controllers.HandleGetUsers(db))
		api.POST("/users", controllers.HandleCreateUser(db))
		api.GET("/logs", controllers.HandleGetLogs(db))

		// LCU API
		api.GET("/lcus", controllers.HandleListLCUsJSON(db))
		api.POST("/lcus", controllers.HandleCreateLCUJSON(db))
		api.GET("/lcus/:id", controllers.HandleGetLCUJSON(db))
		api.POST("/lcus/:id/test", controllers.HandleTestLCU(db, lcuAdapter))
		api.POST("/lcus/:id/sync", controllers.HandleSyncLCU(db, lcuAdapter))
		api.GET("/lcus/:id/lampadaires", controllers.HandleGetLCULampadaires(db))

		// Lampadaires API
		api.GET("/lampadaires/:id", controllers.HandleGetLampadaireJSON(db))
		api.GET("/lampadaires/missing-location", controllers.HandleGetMissingLocationLampadaires(db))
		api.POST("/lampadaires/:id/location", controllers.HandleUpdateLampadaireLocation(db))
		api.POST("/lampadaires/:id/commissioning", controllers.HandleUpdateCommissioningStatus(db))
		api.GET("/lampadaires/:id/diagnostic", controllers.HandleDiagnoseLampadaire(db))

		// Lighting Profiles API
		api.GET("/lighting-profiles", controllers.HandleGetLightingProfiles(db))
		api.POST("/lighting-profiles", controllers.HandleCreateLightingProfile(db))
		api.POST("/lighting-profiles/:id/apply", controllers.HandleApplyLightingProfile(db, lcuAdapter))
		api.GET("/lighting-profiles/:id/details", controllers.HandleGetLightingProfileDetails(db))
		api.POST("/lighting-profiles/:id/enable", controllers.HandleEnableLightingProfile(db))
		api.POST("/lighting-profiles/:id/disable", controllers.HandleDisableLightingProfile(db))

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
		api.POST("/alerts/:id/resolve", controllers.HandleResolveAlert(db))

		// Calculator API
		api.POST("/calculateur/run/:id", controllers.HandleRunCalculator(db, lcuAdapter))
		api.POST("/calculateur/run-all", controllers.HandleRunCalculatorAll(db, lcuAdapter))
		api.GET("/lampadaires/:id/decisions", controllers.HandleGetDecisions(db))

		// Dashboard API
		api.GET("/dashboard/stats", controllers.HandleGetDashboardStats(db))

		// Energy API
		api.GET("/energy/summary", controllers.HandleGetEnergySummary(db))

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
		api.POST("/workorders", controllers.HandleCreateWorkOrder(db))
		api.GET("/workorders/:id", controllers.HandleGetWorkOrder(db))
		api.POST("/workorders/from-alerts", controllers.HandleCreateWorkOrderFromAlerts(db))
		api.POST("/workorders/:id/assign", controllers.HandleAssignWorkOrder(db))
		api.POST("/workorders/:id/start", controllers.HandleStartWorkOrder(db))
		api.POST("/workorders/:id/resolve", controllers.HandleResolveWorkOrder(db))
		api.POST("/workorders/:id/close", controllers.HandleCloseWorkOrder(db))

		// Alerts extended
		api.POST("/alerts/:id/ack", controllers.HandleAckAlert(db))
		api.POST("/alerts/:id/close", controllers.HandleCloseAlert(db))

		// Dashboard extended
		api.GET("/dashboard/network-health", controllers.HandleGetNetworkHealth(db))
		api.GET("/dashboard/commissioning-progress", controllers.HandleGetCommissioningProgress(db))

		// Commissioning workflow
		api.POST("/commissioning/:id/advance", controllers.HandleAdvanceCommissioning(db))
		api.POST("/commissioning/:id/test-comm", controllers.HandleTestCommCommissioning(db))
		api.POST("/commissioning/:id/test-dimming", controllers.HandleTestDimmingCommissioning(db))
		api.POST("/commissioning/:id/validate", controllers.HandleValidateCommissioning(db))
		api.POST("/commissioning/:id/fail", controllers.HandleFailCommissioning(db))
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
