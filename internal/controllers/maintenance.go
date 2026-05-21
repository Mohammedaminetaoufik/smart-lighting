package controllers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/services"
)

// MaintenanceWindow is the full wire representation.
type MaintenanceWindow struct {
	ID                     int        `json:"id"`
	Title                  string     `json:"title,omitempty"`
	MaintenanceType        string     `json:"maintenance_type"`
	TargetType             string     `json:"target_type"`
	TargetID               *int       `json:"target_id,omitempty"`
	TargetReference        string     `json:"target_reference,omitempty"`
	Zone                   string     `json:"zone,omitempty"`
	LampadaireIDs          []int      `json:"lampadaire_ids,omitempty"`
	StartAt                time.Time  `json:"start_at"`
	EndAt                  time.Time  `json:"end_at"`
	Reason                 string     `json:"reason,omitempty"`
	ImpactLevel            string     `json:"impact_level"`
	SuppressAlerts         bool       `json:"suppress_alerts"`
	SuppressAutoWorkOrders bool       `json:"suppress_auto_work_orders"`
	CreateWorkOrder        bool       `json:"create_work_order"`
	Status                 string     `json:"status"`
	RelatedWorkOrderID     *int       `json:"related_work_order_id,omitempty"`
	CreatedBy              *int       `json:"created_by,omitempty"`
	CreatedByName          string     `json:"created_by_name,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	CancelledAt            *time.Time `json:"cancelled_at,omitempty"`
	CompletedAt            *time.Time `json:"completed_at,omitempty"`
}

// scanMaintenanceWindow scans a row into a MaintenanceWindow.
func scanMaintenanceWindow(row interface {
	Scan(...any) error
}) (MaintenanceWindow, error) {
	var w MaintenanceWindow
	var targetID, createdBy, relatedWO sql.NullInt64
	var idsRaw, targetRef, zone sql.NullString
	var cancelledAt, completedAt sql.NullTime

	err := row.Scan(
		&w.ID, &w.Title, &w.MaintenanceType, &w.TargetType,
		&targetID, &targetRef, &zone, &idsRaw,
		&w.StartAt, &w.EndAt, &w.Reason,
		&w.ImpactLevel, &w.SuppressAlerts, &w.SuppressAutoWorkOrders,
		&w.CreateWorkOrder, &w.Status,
		&relatedWO, &createdBy, &w.CreatedByName,
		&w.CreatedAt, &w.UpdatedAt, &cancelledAt, &completedAt,
	)
	if err != nil {
		return w, err
	}
	if targetID.Valid {
		v := int(targetID.Int64)
		w.TargetID = &v
	}
	if targetRef.Valid {
		w.TargetReference = targetRef.String
	}
	if zone.Valid {
		w.Zone = zone.String
	}
	if idsRaw.Valid && idsRaw.String != "" && idsRaw.String != "null" {
		_ = json.Unmarshal([]byte(idsRaw.String), &w.LampadaireIDs)
	}
	if relatedWO.Valid {
		v := int(relatedWO.Int64)
		w.RelatedWorkOrderID = &v
	}
	if createdBy.Valid {
		v := int(createdBy.Int64)
		w.CreatedBy = &v
	}
	if cancelledAt.Valid {
		w.CancelledAt = &cancelledAt.Time
	}
	if completedAt.Valid {
		w.CompletedAt = &completedAt.Time
	}
	return w, nil
}

const maintenanceSelectFields = `
	m.id, COALESCE(m.title,''), COALESCE(m.maintenance_type,'preventive'),
	COALESCE(m.target_type,'zone'), m.target_id, m.target_reference,
	m.zone, m.lampadaire_ids,
	m.start_at, m.end_at, COALESCE(m.reason,''),
	COALESCE(m.impact_level,'low'), m.suppress_alerts, m.suppress_auto_work_orders,
	m.create_work_order, COALESCE(m.status,'planned'),
	m.related_work_order_id, m.created_by, COALESCE(u.full_name,''),
	m.created_at, m.updated_at, m.cancelled_at, m.completed_at
`

const maintenanceJoin = `
	FROM maintenance_windows m
	LEFT JOIN users u ON u.id = m.created_by
`

// HandleGetMaintenanceWindows handles GET /api/maintenance-windows
// Query params: status, from, to
func HandleGetMaintenanceWindows(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		where := []string{"1=1"}
		args := []interface{}{}
		idx := 1

		if s := c.Query("status"); s != "" {
			where = append(where, "m.status=$"+strconv.Itoa(idx))
			args = append(args, s)
			idx++
		}
		if f := c.Query("from"); f != "" {
			where = append(where, "m.end_at >= $"+strconv.Itoa(idx))
			args = append(args, f)
			idx++
		}
		if t := c.Query("to"); t != "" {
			where = append(where, "m.start_at <= $"+strconv.Itoa(idx))
			args = append(args, t)
			idx++
		}
		_ = idx

		query := "SELECT " + maintenanceSelectFields + maintenanceJoin +
			"WHERE " + strings.Join(where, " AND ") +
			" ORDER BY m.start_at DESC LIMIT 200"

		rows, err := db.QueryContext(c.Request.Context(), query, args...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		windows := []MaintenanceWindow{}
		for rows.Next() {
			w, err := scanMaintenanceWindow(rows)
			if err != nil {
				continue
			}
			windows = append(windows, w)
		}
		RespondJSON(c, http.StatusOK, windows)
	}
}

// HandleGetMaintenanceWindow handles GET /api/maintenance-windows/:id
func HandleGetMaintenanceWindow(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}
		row := db.QueryRowContext(c.Request.Context(),
			"SELECT "+maintenanceSelectFields+maintenanceJoin+"WHERE m.id=$1", id)
		w, err := scanMaintenanceWindow(row)
		if err == sql.ErrNoRows {
			RespondError(c, http.StatusNotFound, "Fenêtre introuvable")
			return
		}
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		RespondJSON(c, http.StatusOK, w)
	}
}

// HandleGetActiveMaintenanceWindows handles GET /api/maintenance-windows/active
func HandleGetActiveMaintenanceWindows(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.QueryContext(c.Request.Context(),
			"SELECT "+maintenanceSelectFields+maintenanceJoin+
				"WHERE m.start_at <= NOW() AND m.end_at >= NOW() AND m.status IN ('planned','active') "+
				"ORDER BY m.start_at ASC")
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		windows := []MaintenanceWindow{}
		for rows.Next() {
			w, err := scanMaintenanceWindow(rows)
			if err != nil {
				continue
			}
			windows = append(windows, w)
		}
		RespondJSON(c, http.StatusOK, windows)
	}
}

// HandleGetUpcomingMaintenanceWindows handles GET /api/maintenance-windows/upcoming
func HandleGetUpcomingMaintenanceWindows(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.QueryContext(c.Request.Context(),
			"SELECT "+maintenanceSelectFields+maintenanceJoin+
				"WHERE m.start_at > NOW() AND m.status='planned' "+
				"ORDER BY m.start_at ASC LIMIT 20")
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		windows := []MaintenanceWindow{}
		for rows.Next() {
			w, err := scanMaintenanceWindow(rows)
			if err != nil {
				continue
			}
			windows = append(windows, w)
		}
		RespondJSON(c, http.StatusOK, windows)
	}
}

const sqlLinkWorkOrder = "UPDATE maintenance_windows SET related_work_order_id=$1 WHERE id=$2"

// ensureMaintenanceSourceConstraint makes 'maintenance_window' a valid source_type (idempotent).
func ensureMaintenanceSourceConstraint(db *sql.DB) {
	db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_work_order_source_v3') THEN
				ALTER TABLE work_orders DROP CONSTRAINT IF EXISTS chk_work_order_source_v2;
				ALTER TABLE work_orders ADD CONSTRAINT chk_work_order_source_v3
					CHECK (source_type IN ('alert','manual','system','calculator','maintenance_window'));
			END IF;
		END $$;
	`)
}

// createLinkedWorkOrder inserts a work_order tied to a maintenance window and links it back.
// Returns (workOrderID, error). On success it also writes related_work_order_id on the window.
func createLinkedWorkOrder(db *sql.DB, ac services.AuditCtx, windowID int, title, reason, zone string) (int, error) {
	ensureMaintenanceSourceConstraint(db)

	var woID int
	err := db.QueryRow(`
		INSERT INTO work_orders
			(title, description, status, source_type, maintenance_window_id,
			 zone, equipment_type, created_by, updated_at)
		VALUES ($1, $2, 'open', 'maintenance_window', $3, NULLIF($4,''), 'lampadaire', $5, NOW())
		RETURNING id`,
		firstNonEmpty(title, "Maintenance #"+strconv.Itoa(windowID)),
		reason, windowID, zone, ac.UserID,
	).Scan(&woID)
	if err != nil {
		return 0, err
	}
	db.Exec(sqlLinkWorkOrder, woID, windowID)
	return woID, nil
}

// HandleCreateMaintenanceWindow handles POST /api/maintenance-windows
func HandleCreateMaintenanceWindow(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Title                  string    `json:"title"`
			MaintenanceType        string    `json:"maintenance_type"`
			TargetType             string    `json:"target_type"`
			TargetID               *int      `json:"target_id"`
			TargetReference        string    `json:"target_reference"`
			Zone                   string    `json:"zone"`
			LampadaireIDs          []int     `json:"lampadaire_ids"`
			StartAt                time.Time `json:"start_at"`
			EndAt                  time.Time `json:"end_at"`
			Reason                 string    `json:"reason"`
			ImpactLevel            string    `json:"impact_level"`
			SuppressAlerts         bool      `json:"suppress_alerts"`
			SuppressAutoWorkOrders bool      `json:"suppress_auto_work_orders"`
			CreateWorkOrder        bool      `json:"create_work_order"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}

		if body.StartAt.IsZero() || body.EndAt.IsZero() {
			RespondError(c, http.StatusBadRequest, "start_at et end_at requis")
			return
		}
		if !body.EndAt.After(body.StartAt) {
			RespondError(c, http.StatusBadRequest, "end_at doit être après start_at")
			return
		}

		body.MaintenanceType = firstNonEmpty(body.MaintenanceType, "preventive")
		body.TargetType = firstNonEmpty(body.TargetType, "zone")
		body.ImpactLevel = firstNonEmpty(body.ImpactLevel, "low")

		if body.TargetType == "zone" && strings.TrimSpace(body.Zone) == "" {
			RespondError(c, http.StatusBadRequest, "zone requise pour ce type de cible")
			return
		}
		if body.TargetType == "selection" && len(body.LampadaireIDs) == 0 {
			RespondError(c, http.StatusBadRequest, "lampadaire_ids requis pour le type sélection")
			return
		}
		if (body.TargetType == "lcu" || body.TargetType == "lampadaire") && body.TargetID == nil {
			RespondError(c, http.StatusBadRequest, "target_id requis pour ce type de cible")
			return
		}

		var idsJSON []byte
		if len(body.LampadaireIDs) > 0 {
			idsJSON, _ = json.Marshal(body.LampadaireIDs)
		}

		// Determine initial status
		now := time.Now()
		status := "planned"
		if !body.StartAt.After(now) && body.EndAt.After(now) {
			status = "active"
		}

		ac := services.GetAuditContext(c)

		var id int
		err := db.QueryRowContext(c.Request.Context(), `
			INSERT INTO maintenance_windows
				(title, maintenance_type, target_type, target_id, target_reference,
				 zone, lampadaire_ids, start_at, end_at, reason,
				 impact_level, suppress_alerts, suppress_auto_work_orders,
				 create_work_order, status, created_by, updated_at)
			VALUES (NULLIF($1,''), $2, $3, $4, NULLIF($5,''),
			        NULLIF($6,''), NULLIF($7,'')::jsonb, $8, $9, NULLIF($10,''),
			        $11, $12, $13, $14, $15, $16, NOW())
			RETURNING id`,
			body.Title, body.MaintenanceType, body.TargetType, body.TargetID, body.TargetReference,
			body.Zone, string(idsJSON), body.StartAt, body.EndAt, body.Reason,
			body.ImpactLevel, body.SuppressAlerts, body.SuppressAutoWorkOrders,
			body.CreateWorkOrder, status, ac.UserID,
		).Scan(&id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur création: "+err.Error())
			return
		}

		// Optionally create a linked work order
		if body.CreateWorkOrder {
			woTitle := firstNonEmpty(body.Title, "Maintenance: "+body.MaintenanceType)

			// Ensure constraint allows 'maintenance_window' (idempotent)
			db.ExecContext(c.Request.Context(), `
				DO $$
				BEGIN
					IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_work_order_source_v3') THEN
						ALTER TABLE work_orders DROP CONSTRAINT IF EXISTS chk_work_order_source_v2;
						ALTER TABLE work_orders ADD CONSTRAINT chk_work_order_source_v3
							CHECK (source_type IN ('alert','manual','system','calculator','maintenance_window'));
					END IF;
				END $$;
			`)

			var woID int
			woErr := db.QueryRowContext(c.Request.Context(), `
				INSERT INTO work_orders
					(title, description, status, source_type, maintenance_window_id,
					 zone, equipment_type, created_by, updated_at)
				VALUES ($1, $2, 'open', 'maintenance_window', $3, NULLIF($4,''), 'lampadaire', $5, NOW())
				RETURNING id`,
				woTitle, body.Reason, id, body.Zone, ac.UserID,
			).Scan(&woID)
			if woErr != nil {
				log.Printf("maintenance: work order creation failed for window #%d: %v", id, woErr)
				RespondJSON(c, http.StatusCreated, gin.H{
					"id": id, "status": status,
					"warning": "Fenêtre créée mais la création de l'OT a échoué: " + woErr.Error(),
				})
				return
			}
			db.ExecContext(c.Request.Context(),
				sqlLinkWorkOrder, woID, id)
			services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
				UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
				Action: "work_order_created", EntityType: "work_order", EntityID: &woID,
				Description: "Ordre de travail créé depuis fenêtre de maintenance #" + strconv.Itoa(id),
				IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
			})
		}

		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "maintenance_window_created", EntityType: "maintenance_window", EntityID: &id,
			Description: "Fenêtre de maintenance créée: " + firstNonEmpty(body.Title, body.MaintenanceType),
			NewValues: map[string]any{
				"maintenance_type": body.MaintenanceType,
				"target_type":      body.TargetType,
				"start_at":         body.StartAt,
				"end_at":           body.EndAt,
				"impact_level":     body.ImpactLevel,
			},
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})

		RespondJSON(c, http.StatusCreated, gin.H{"id": id, "status": status})
	}
}

// HandleUpdateMaintenanceWindow handles PUT /api/maintenance-windows/:id
func HandleUpdateMaintenanceWindow(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}

		var body struct {
			Title                  string    `json:"title"`
			MaintenanceType        string    `json:"maintenance_type"`
			StartAt                time.Time `json:"start_at"`
			EndAt                  time.Time `json:"end_at"`
			Reason                 string    `json:"reason"`
			ImpactLevel            string    `json:"impact_level"`
			SuppressAlerts         *bool     `json:"suppress_alerts"`
			SuppressAutoWorkOrders *bool     `json:"suppress_auto_work_orders"`
			CreateWorkOrder        *bool     `json:"create_work_order"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if !body.EndAt.IsZero() && !body.StartAt.IsZero() && !body.EndAt.After(body.StartAt) {
			RespondError(c, http.StatusBadRequest, "end_at doit être après start_at")
			return
		}

		ac := services.GetAuditContext(c)

		// Fetch current state so we can decide whether to create a work order
		var curTitle, curType, curZone, curReason sql.NullString
		var curRelatedWO sql.NullInt64
		var curCreateWO bool
		db.QueryRowContext(c.Request.Context(),
			`SELECT COALESCE(title,''), COALESCE(maintenance_type,'preventive'),
			        COALESCE(zone,''), COALESCE(reason,''),
			        related_work_order_id, create_work_order
			 FROM maintenance_windows WHERE id=$1`, id).
			Scan(&curTitle, &curType, &curZone, &curReason, &curRelatedWO, &curCreateWO)

		res, err := db.ExecContext(c.Request.Context(), `
			UPDATE maintenance_windows SET
				title = COALESCE(NULLIF($1,''), title),
				maintenance_type = COALESCE(NULLIF($2,''), maintenance_type),
				start_at = CASE WHEN $3::timestamptz IS NOT NULL AND $3 != '0001-01-01' THEN $3 ELSE start_at END,
				end_at   = CASE WHEN $4::timestamptz IS NOT NULL AND $4 != '0001-01-01' THEN $4 ELSE end_at END,
				reason   = COALESCE(NULLIF($5,''), reason),
				impact_level = COALESCE(NULLIF($6,''), impact_level),
				suppress_alerts = COALESCE($7, suppress_alerts),
				suppress_auto_work_orders = COALESCE($8, suppress_auto_work_orders),
				create_work_order = COALESCE($10, create_work_order),
				updated_at = NOW()
			WHERE id=$9 AND status NOT IN ('completed','cancelled')`,
			body.Title, body.MaintenanceType, nilIfZeroTime(body.StartAt), nilIfZeroTime(body.EndAt),
			body.Reason, body.ImpactLevel, body.SuppressAlerts, body.SuppressAutoWorkOrders, id,
			body.CreateWorkOrder)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			RespondError(c, http.StatusNotFound, "Fenêtre introuvable ou déjà terminée")
			return
		}

		// Create linked work order if the flag was just turned on and none exists yet
		wantWO := body.CreateWorkOrder != nil && *body.CreateWorkOrder
		hasWO := curRelatedWO.Valid
		if wantWO && !hasWO {
			woTitle := firstNonEmpty(body.Title, curTitle.String, "Maintenance: "+curType.String)
			woReason := firstNonEmpty(body.Reason, curReason.String)
			woZone := curZone.String // zone from the pre-fetch

			// Ensure 'maintenance_window' is an accepted source_type (idempotent)
			db.ExecContext(c.Request.Context(), `
				DO $$
				BEGIN
					IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_work_order_source_v3') THEN
						ALTER TABLE work_orders DROP CONSTRAINT IF EXISTS chk_work_order_source_v2;
						ALTER TABLE work_orders ADD CONSTRAINT chk_work_order_source_v3
							CHECK (source_type IN ('alert','manual','system','calculator','maintenance_window'));
					END IF;
				END $$;
			`)

			var woID int
			woErr := db.QueryRowContext(c.Request.Context(), `
				INSERT INTO work_orders
					(title, description, status, source_type, maintenance_window_id,
					 zone, equipment_type, created_by, updated_at)
				VALUES ($1, $2, 'open', 'maintenance_window', $3, NULLIF($4,''), 'lampadaire', $5, NOW())
				RETURNING id`,
				woTitle, woReason, id, woZone, ac.UserID,
			).Scan(&woID)
			if woErr != nil {
				log.Printf("maintenance: work order creation failed for window #%d: %v", id, woErr)
				RespondJSON(c, http.StatusOK, gin.H{
					"status": "updated", "id": id,
					"warning": "Fenêtre mise à jour mais la création de l'OT a échoué: " + woErr.Error(),
				})
				return
			}
			db.ExecContext(c.Request.Context(),
				sqlLinkWorkOrder, woID, id)
			services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
				UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
				Action: "work_order_created", EntityType: "work_order", EntityID: &woID,
				Description: "Ordre de travail créé depuis fenêtre de maintenance #" + strconv.Itoa(id),
				IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
			})
		}

		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "maintenance_window_updated", EntityType: "maintenance_window", EntityID: &id,
			Description: "Fenêtre de maintenance mise à jour",
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "updated", "id": id})
	}
}

// HandleCancelMaintenanceWindow handles POST /api/maintenance-windows/:id/cancel
func HandleCancelMaintenanceWindow(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}
		ac := services.GetAuditContext(c)

		res, err := db.ExecContext(c.Request.Context(), `
			UPDATE maintenance_windows
			SET status='cancelled', cancelled_at=NOW(), updated_at=NOW()
			WHERE id=$1 AND status NOT IN ('completed','cancelled')`, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			RespondError(c, http.StatusNotFound, "Fenêtre introuvable ou déjà terminée/annulée")
			return
		}

		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "maintenance_window_cancelled", EntityType: "maintenance_window", EntityID: &id,
			Description: "Fenêtre de maintenance annulée",
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "cancelled", "id": id})
	}
}

// HandleCompleteMaintenanceWindow handles POST /api/maintenance-windows/:id/complete
func HandleCompleteMaintenanceWindow(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}
		ac := services.GetAuditContext(c)

		res, err := db.ExecContext(c.Request.Context(), `
			UPDATE maintenance_windows
			SET status='completed', completed_at=NOW(), updated_at=NOW()
			WHERE id=$1 AND status NOT IN ('completed','cancelled')`, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			RespondError(c, http.StatusNotFound, "Fenêtre introuvable ou déjà terminée/annulée")
			return
		}

		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "maintenance_window_completed", EntityType: "maintenance_window", EntityID: &id,
			Description: "Fenêtre de maintenance marquée terminée",
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "completed", "id": id})
	}
}

// HandleDeleteMaintenanceWindow handles DELETE /api/maintenance-windows/:id
// Only planned windows that haven't started yet can be deleted.
func HandleDeleteMaintenanceWindow(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}
		ac := services.GetAuditContext(c)

		res, err := db.ExecContext(c.Request.Context(),
			"DELETE FROM maintenance_windows WHERE id=$1", id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			RespondError(c, http.StatusNotFound, "Fenêtre introuvable")
			return
		}

		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "maintenance_window_deleted", EntityType: "maintenance_window", EntityID: &id,
			Description: "Fenêtre de maintenance supprimée",
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "deleted", "id": id})
	}
}

// HandleCheckMaintenance handles GET /api/maintenance-windows/check
// Query params: lampadaire_id, zone, lcu_id
func HandleCheckMaintenance(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		lampID := 0
		if v := c.Query("lampadaire_id"); v != "" {
			lampID, _ = parseIntStr(v)
		}
		zone := c.Query("zone")
		var lcuID *int
		if v := c.Query("lcu_id"); v != "" {
			if n, err := parseIntStr(v); err == nil {
				lcuID = &n
			}
		}

		inMaint, win := services.IsEquipmentInMaintenance(c.Request.Context(), db, lampID, zone, lcuID)
		if !inMaint || win == nil {
			RespondJSON(c, http.StatusOK, gin.H{"in_maintenance": false})
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{
			"in_maintenance":           true,
			"window_id":                win.ID,
			"title":                    win.Title,
			"maintenance_type":         win.MaintenanceType,
			"suppress_alerts":          win.SuppressAlerts,
			"suppress_auto_work_orders": win.SuppressAutoWorkOrders,
			"impact_level":             win.ImpactLevel,
		})
	}
}

// nilIfZeroTime returns nil for the zero time (so SQL CASE won't update).
func nilIfZeroTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseIntStr(s string) (int, error) {
	return strconv.Atoi(s)
}
