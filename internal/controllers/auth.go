package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"map-interactif/internal/middleware"
	"map-interactif/internal/repository"
	"map-interactif/internal/services"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// HandleLogin handles POST /api/auth/login.
func HandleLogin(db *sql.DB) gin.HandlerFunc {
	secret := middleware.JWTSecret()
	return func(c *gin.Context) {
		var req loginRequest
		if !BindRequiredJSON(c, &req) {
			return
		}
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		if req.Email == "" || req.Password == "" {
			RespondError(c, http.StatusBadRequest, "Email et mot de passe requis")
			return
		}

		user, err := repository.FindUserByEmail(c.Request.Context(), db, req.Email)
		if err != nil {
			RespondError(c, http.StatusUnauthorized, "Identifiants invalides")
			return
		}
		if user.Status != "active" {
			RespondError(c, http.StatusForbidden, "Compte désactivé — contactez votre administrateur")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			RespondError(c, http.StatusUnauthorized, "Identifiants invalides")
			return
		}

		expiry := time.Now().Add(8 * time.Hour)
		claims := middleware.AuthClaims{
			Sub:   fmt.Sprintf("%d", user.ID),
			Name:  user.FullName,
			Email: user.Email,
			Role:  user.Role,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(expiry),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString(secret)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur génération token")
			return
		}

		_ = repository.UpdateLastLogin(c.Request.Context(), db, user.ID)
		userID := user.ID
		services.LogAction(c.Request.Context(), db, services.AuditEvent{
			UserID:    &userID,
			Action:    "login",
			IPAddress: c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		})

		RespondJSON(c, http.StatusOK, gin.H{
			"token": tokenStr,
			"user": gin.H{
				"id":    user.ID,
				"name":  user.FullName,
				"email": user.Email,
				"role":  user.Role,
			},
		})
	}
}

// HandleMe handles GET /api/auth/me — returns the authenticated user's info.
func HandleMe() gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondJSON(c, http.StatusOK, gin.H{
			"id":    c.GetString("user_id"),
			"name":  c.GetString("user_name"),
			"email": c.GetString("user_email"),
			"role":  c.GetString("user_role"),
		})
	}
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// HandleChangePassword handles POST /api/auth/change-password.
func HandleChangePassword(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req changePasswordRequest
		if !BindRequiredJSON(c, &req) {
			return
		}
		if len(req.NewPassword) < 8 {
			RespondError(c, http.StatusBadRequest, "Le nouveau mot de passe doit contenir au moins 8 caractères")
			return
		}

		email := c.GetString("user_email")
		user, err := repository.FindUserByEmail(c.Request.Context(), db, email)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur utilisateur")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			RespondError(c, http.StatusUnauthorized, "Mot de passe actuel incorrect")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 10)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur hachage")
			return
		}
		if err := repository.UpdatePassword(c.Request.Context(), db, user.ID, string(hash)); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour mot de passe")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"status": "ok"})
	}
}

type resetPasswordRequest struct {
	UserID      int    `json:"user_id"`
	NewPassword string `json:"new_password"`
}

// HandleAdminResetPassword handles POST /api/auth/admin/reset-password (admin only).
func HandleAdminResetPassword(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req resetPasswordRequest
		if !BindRequiredJSON(c, &req) {
			return
		}
		if req.UserID <= 0 {
			RespondError(c, http.StatusBadRequest, "user_id invalide")
			return
		}
		if len(req.NewPassword) < 8 {
			RespondError(c, http.StatusBadRequest, "Le mot de passe doit contenir au moins 8 caractères")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 10)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur hachage")
			return
		}
		if err := repository.UpdatePassword(c.Request.Context(), db, req.UserID, string(hash)); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour mot de passe")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"status": "ok", "user_id": req.UserID})
	}
}
