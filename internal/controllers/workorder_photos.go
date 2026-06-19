package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// HandleUploadWorkOrderPhoto handles POST /api/mobile/workorders/:id/photos
func HandleUploadWorkOrderPhoto(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		woID, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}

		// Vérifier si le bon de travail existe
		var exists bool
		err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM work_orders WHERE id = $1)", woID).Scan(&exists)
		if err != nil || !exists {
			RespondError(c, http.StatusNotFound, "Bon de travail introuvable")
			return
		}

		technicianIDStr := c.PostForm("technician_id")
		technicianID, _ := strconv.Atoi(technicianIDStr)
		if technicianID <= 0 {
			technicianID = 1 // Défaut
		}

		file, err := c.FormFile("photo")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "Le champ 'photo' est requis")
			return
		}

		// Extensions autorisées
		ext := filepath.Ext(file.Filename)
		valid := false
		for _, v := range []string{".jpg", ".jpeg", ".png", ".webp"} {
			if strings.EqualFold(ext, v) {
				valid = true
				break
			}
		}
		if !valid {
			RespondError(c, http.StatusBadRequest, "Format d'image non supporté (autorisés: jpg, png, webp)")
			return
		}

		// Limite 10MB
		if file.Size > 10<<20 {
			RespondError(c, http.StatusBadRequest, "L'image est trop lourde (max 10Mo)")
			return
		}

		uploadDir := os.Getenv("UPLOAD_DIR")
		if uploadDir == "" {
			uploadDir = "uploads"
		}

		relPath := fmt.Sprintf("workorders/%d", woID)
		fullDir := filepath.Join(uploadDir, relPath)
		if err := os.MkdirAll(fullDir, 0755); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur création dossier upload")
			return
		}

		filename := fmt.Sprintf("%d_%x%s", time.Now().UnixNano(), time.Now().Unix(), ext)
		savePath := filepath.Join(fullDir, filename)
		dbPath := filepath.ToSlash(filepath.Join(relPath, filename))

		if err := c.SaveUploadedFile(file, savePath); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur sauvegarde fichier")
			return
		}

		_, err = db.Exec(`
			INSERT INTO work_order_photos (work_order_id, technician_id, file_path)
			VALUES ($1, $2, $3)
		`, woID, technicianID, dbPath)

		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur enregistrement base de données")
			return
		}

		RespondJSON(c, http.StatusCreated, gin.H{
			"message": "Photo uploadée avec succès",
			"path":    dbPath,
			"url":     "/uploads/" + dbPath,
		})
	}
}

// HandleListWorkOrderPhotos handles GET /api/mobile/workorders/:id/photos
func HandleListWorkOrderPhotos(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		woID, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}

		rows, err := db.Query(`
			SELECT id, file_path, created_at
			FROM work_order_photos
			WHERE work_order_id = $1
			ORDER BY created_at DESC
		`, woID)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errDB)
			return
		}
		defer rows.Close()

		type Photo struct {
			ID        int       `json:"id"`
			FilePath  string    `json:"file_path"`
			URL       string    `json:"url"`
			CreatedAt time.Time `json:"created_at"`
		}

		var photos []Photo
		for rows.Next() {
			var p Photo
			if err := rows.Scan(&p.ID, &p.FilePath, &p.CreatedAt); err != nil {
				continue
			}
			p.URL = "/uploads/" + p.FilePath
			photos = append(photos, p)
		}

		if photos == nil {
			photos = []Photo{}
		}

		RespondJSON(c, http.StatusOK, photos)
	}
}
