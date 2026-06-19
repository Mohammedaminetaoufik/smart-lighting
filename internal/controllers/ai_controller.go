package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// HandleAISuggestions proxies GET /ai/suggestions to FastAPI.
func HandleAISuggestions() gin.HandlerFunc {
	return func(c *gin.Context) {
		url := getAIServiceURL() + "/ai/suggestions"
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}
		proxyRequest(c, req)
	}
}

// HandleAIDailyDigest proxies GET /ai/daily-digest to FastAPI.
func HandleAIDailyDigest() gin.HandlerFunc {
	return func(c *gin.Context) {
		targetURL := getAIServiceURL() + "/ai/daily-digest"
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

// HandleAIQueryStream proxies POST /ai/query/stream to FastAPI and pipes the SSE stream back to the client.
func HandleAIQueryStream() gin.HandlerFunc {
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

		targetURL := getAIServiceURL() + "/ai/query/stream"
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")

		// Use a dedicated streaming client with no timeout (stream runs until [DONE])
		streamClient := &http.Client{}
		resp, err := streamClient.Do(req)
		if err != nil {
			RespondError(c, http.StatusBadGateway, fmt.Sprintf("Service IA indisponible: %s", err.Error()))
			return
		}
		defer resp.Body.Close()

		// Forward SSE headers
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Accel-Buffering", "no")
		c.Status(resp.StatusCode)

		// Pipe the SSE stream directly to the client, flushing each chunk
		flusher, canFlush := c.Writer.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
					return
				}
				if canFlush {
					flusher.Flush()
				}
			}
			if readErr != nil {
				return
			}
		}
	}
}

// HandleAIDecisionCenter proxies GET /ai/decision-center to FastAPI.
func HandleAIDecisionCenter() gin.HandlerFunc {
	return func(c *gin.Context) {
		targetURL := getAIServiceURL() + "/ai/decision-center"
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

// HandleAIMobileMissions proxies GET /ai/mobile/missions to FastAPI.
func HandleAIMobileMissions() gin.HandlerFunc {
	return func(c *gin.Context) {
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet,
			getAIServiceURL()+"/ai/mobile/missions", nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}
		proxyRequest(c, req)
	}
}

// HandleAIMobileDiagnosticLampadaire proxies GET /ai/mobile/lampadaires/:id/diagnostic to FastAPI.
func HandleAIMobileDiagnosticLampadaire() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		targetURL := fmt.Sprintf("%s/ai/mobile/lampadaires/%s/diagnostic", getAIServiceURL(), id)
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, targetURL, nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}
		proxyRequest(c, req)
	}
}

// HandleAIMobileDiagnosticLCU proxies GET /ai/mobile/lcus/:id/diagnostic to FastAPI.
func HandleAIMobileDiagnosticLCU() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		targetURL := fmt.Sprintf("%s/ai/mobile/lcus/%s/diagnostic", getAIServiceURL(), id)
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, targetURL, nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}
		proxyRequest(c, req)
	}
}

// HandleAIMobileDiagnosticWorkOrder proxies GET /ai/mobile/workorders/:id/diagnostic to FastAPI.
func HandleAIMobileDiagnosticWorkOrder() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		targetURL := fmt.Sprintf("%s/ai/mobile/workorders/%s/diagnostic", getAIServiceURL(), id)
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

		search := c.Query("search")
		targetURL := fmt.Sprintf("%s/ai/history?limit=%d", getAIServiceURL(), limit)
		if search != "" {
			targetURL += "&search=" + url.QueryEscape(search)
		}

		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, targetURL, nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, errAIRequestCreation)
			return
		}

		proxyRequest(c, req)
	}
}
