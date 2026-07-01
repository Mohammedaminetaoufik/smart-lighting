package controllers

import (
	"database/sql"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
	"map-interactif/internal/services"
)

var (
	emailRegex        = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	validRoles        = map[string]bool{"admin": true, "operator": true, "viewer": true}
	validUserStatuses = map[string]bool{"active": true, "disabled": true}
)

func validateUserPayload(u *models.User) string {
	u.FullName = strings.TrimSpace(u.FullName)
	u.Email = strings.TrimSpace(strings.ToLower(u.Email))
	u.Role = strings.TrimSpace(u.Role)
	u.Status = strings.TrimSpace(u.Status)

	if u.FullName == "" {
		return "Le nom est obligatoire"
	}
	if u.Email == "" || !emailRegex.MatchString(u.Email) {
		return "Email invalide"
	}
	if u.Role == "" {
		u.Role = "viewer"
	}
	if !validRoles[u.Role] {
		return "Rôle invalide (admin, operator, viewer)"
	}
	if u.Status == "" {
		u.Status = "active"
	}
	if !validUserStatuses[u.Status] {
		return "Statut invalide (active, disabled)"
	}
	return ""
}

// mergeUserUpdates fills empty fields in payload with values from existing.
func mergeUserUpdates(payload, existing *models.User) {
	if payload.FullName == "" {
		payload.FullName = existing.FullName
	}
	if payload.Email == "" {
		payload.Email = existing.Email
	}
	if payload.Role == "" {
		payload.Role = existing.Role
	}
	if payload.Status == "" {
		payload.Status = existing.Status
	}
}

// checkEmailAvailable verifies the email isn't taken. Writes 409/500 and returns
// false if the request should not proceed.
func checkEmailAvailable(c *gin.Context, db *sql.DB, email string, excludeID int) bool {
	exists, err := repository.EmailExists(c.Request.Context(), db, email, excludeID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Erreur vérification email")
		return false
	}
	if exists {
		RespondError(c, http.StatusConflict, "Cet email est déjà utilisé")
		return false
	}
	return true
}

// HandleGetUsers handles GET /api/users.
func HandleGetUsers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := repository.ListUsers(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		RespondJSON(c, http.StatusOK, users)
	}
}

// HandleGetUser handles GET /api/users/:id.
func HandleGetUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}
		u, err := repository.GetUserByID(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Utilisateur introuvable")
			return
		}
		RespondJSON(c, http.StatusOK, u)
	}
}

type createUserRequest struct {
	models.User
	Password string `json:"password"`
}

// HandleCreateUser handles POST /api/users.
func HandleCreateUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createUserRequest
		if !BindRequiredJSON(c, &req) {
			return
		}
		if req.Password == "" {
			RespondError(c, http.StatusBadRequest, "Le mot de passe est obligatoire")
			return
		}
		if len(req.Password) < 8 {
			RespondError(c, http.StatusBadRequest, "Le mot de passe doit contenir au moins 8 caractères")
			return
		}
		if msg := validateUserPayload(&req.User); msg != "" {
			RespondError(c, http.StatusBadRequest, msg)
			return
		}
		if !checkEmailAvailable(c, db, req.Email, 0) {
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur hachage mot de passe")
			return
		}
		req.User.PasswordHash = string(hash)
		id, err := repository.InsertUser(c.Request.Context(), db, req.User)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur création: "+err.Error())
			return
		}
		req.User.ID = id
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			Action: "user_created", EntityType: "user", EntityID: &id,
			Description: "Utilisateur créé : " + req.Email,
			NewValues: map[string]any{"email": req.Email, "role": req.Role, "status": req.Status},
			IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		})
		RespondJSON(c, http.StatusCreated, req.User)
	}
}

// HandleUpdateUser handles PATCH /api/users/:id.
func HandleUpdateUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}
		existing, err := repository.GetUserByID(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Utilisateur introuvable")
			return
		}
		var payload models.User
		if !BindRequiredJSON(c, &payload) {
			return
		}
		mergeUserUpdates(&payload, existing)
		if msg := validateUserPayload(&payload); msg != "" {
			RespondError(c, http.StatusBadRequest, msg)
			return
		}
		if payload.Email != existing.Email && !checkEmailAvailable(c, db, payload.Email, id) {
			return
		}
		if err := repository.UpdateUser(c.Request.Context(), db, id,
			payload.FullName, payload.Email, payload.Role, payload.Status); err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			Action: "user_updated", EntityType: "user", EntityID: &id,
			Description: "Utilisateur modifié",
			OldValues: map[string]any{"role": existing.Role, "status": existing.Status},
			NewValues: map[string]any{"role": payload.Role, "status": payload.Status},
			IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		})
		u, _ := repository.GetUserByID(c.Request.Context(), db, id)
		RespondJSON(c, http.StatusOK, u)
	}
}

// HandleDeleteUser handles DELETE /api/users/:id (soft delete).
func HandleDeleteUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}
		if err := repository.SoftDeleteUser(c.Request.Context(), db, id); err != nil {
			RespondError(c, http.StatusNotFound, err.Error())
			return
		}
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			Action: "user_deleted", EntityType: "user", EntityID: &id,
			Description: "Utilisateur supprimé (soft delete)",
			NewValues: map[string]any{"status": "deleted"},
			IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "deleted", "id": id})
	}
}
