package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const errAIRequestCreation = "Erreur création requête IA"

func getAIServiceURL() string {
	if url := os.Getenv("AI_SERVICE_URL"); url != "" {
		return url
	}
	return "http://localhost:8090"
}

var aiHTTPClient = &http.Client{
	Timeout: 60 * time.Second,
}

// proxyRequest forwards a request to FastAPI and writes the response back to Gin.
// It preserves the FastAPI status code.
func proxyRequest(c *gin.Context, req *http.Request) {
	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		RespondError(c, http.StatusBadGateway, fmt.Sprintf("Service IA indisponible: %s", err.Error()))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Erreur lecture réponse IA")
		return
	}

	c.Data(resp.StatusCode, "application/json; charset=utf-8", body)
}

// HandleAIHealth proxies GET /health from FastAPI.
func HandleAIHealth() gin.HandlerFunc {
	return func(c *gin.Context) {
		url := getAIServiceURL() + "/health"
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}
		proxyRequest(c, req)
	}
}

// AIQueryRequest is the payload expected by POST /api/ai/query.
type AIQueryRequest struct {
	Question string `json:"question"`
	Language string `json:"language"`
	MaxRows  int    `json:"max_rows"`
}

// HandleAIQuery proxies POST /ai/query to FastAPI.
func HandleAIQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload AIQueryRequest
		if !BindRequiredJSON(c, &payload) {
			return
		}

		if payload.Question == "" {
			RespondError(c, http.StatusBadRequest, "Le champ 'question' est obligatoire")
			return
		}
		if payload.Language == "" {
			payload.Language = "fr"
		}
		if payload.MaxRows <= 0 {
			payload.MaxRows = 100
		}

		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur sérialisation requête")
			return
		}

		url := getAIServiceURL() + "/ai/query"
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		proxyRequest(c, req)
	}
}

// HandleAIEntityInsights proxies GET /ai/entity-insights/:entityType/:entityId to FastAPI.
// TODO: add authentication middleware before this handler in production.
func HandleAIEntityInsights() gin.HandlerFunc {
	return func(c *gin.Context) {
		entityType := c.Param("entityType")
		entityID := c.Param("entityId")
		if entityType == "" || entityID == "" {
			RespondError(c, http.StatusBadRequest, "Les paramètres 'entityType' et 'entityId' sont requis")
			return
		}

		targetURL := fmt.Sprintf("%s/ai/entity-insights/%s/%s", getAIServiceURL(), entityType, entityID)
		if c.Query("refresh") == "true" {
			targetURL += "?refresh=true"
		}

		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, targetURL, nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}

		proxyRequest(c, req)
	}
}

// HandleAIPageInsights proxies GET /ai/page-insights/:page to FastAPI.
// TODO: add authentication middleware before this handler in production.
func HandleAIPageInsights() gin.HandlerFunc {
	return func(c *gin.Context) {
		page := c.Param("page")
		if page == "" {
			RespondError(c, http.StatusBadRequest, "Le paramètre 'page' est requis")
			return
		}

		refresh := c.Query("refresh")
		targetURL := fmt.Sprintf("%s/ai/page-insights/%s", getAIServiceURL(), page)
		if refresh == "true" {
			targetURL += "?refresh=true"
		}

		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, targetURL, nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}

		proxyRequest(c, req)
	}
}

// HandleAIHistory proxies GET /ai/history to FastAPI.
func HandleAIHistory() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 20
		if raw := c.Query("limit"); raw != "" {
			if v, err := strconv.Atoi(raw); err == nil {
				if v < 1 {
					v = 1
				}
				if v > 100 {
					v = 100
				}
				limit = v
			}
		}

		url := fmt.Sprintf("%s/ai/history?limit=%d", getAIServiceURL(), limit)
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}

		proxyRequest(c, req)
	}
}
