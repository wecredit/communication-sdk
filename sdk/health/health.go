package health

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/internal/queue"
	"github.com/wecredit/communication-sdk/sdk/pkg/cache"
)

type HealthCheckResponse struct {
	Status         string `json:"status"`
	TechDB         string `json:"tech_db"`
	CacheStatus    string `json:"cache_status,omitempty"`
	AWSQueueClient string `json:"aws_queue_client"`
	ClientIP       string `json:"client_ip"`
	ServerPort     string `json:"server_port"`
}

// HealthCheckHandler handles health checks using Gin
func HealthCheckHandler(port string) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := HealthCheckResponse{
			Status:     "ok",
			ClientIP:   c.ClientIP(),
			ServerPort: port,
		}

		// Check TechDB
		if err := database.PingTechDB(); err != nil {
			resp.TechDB = "unhealthy: " + err.Error()
			resp.Status = "degraded"
		} else {
			resp.TechDB = "healthy"
		}

		// Check AWS Queue clients
		if queue.SQSClient == nil || queue.SNSClient == nil {
			resp.AWSQueueClient = "unhealthy: AWS clients not initialized"
			resp.Status = "degraded"
		} else {
			resp.AWSQueueClient = "healthy"
		}

		// Check cache
		if cache.GetCache() == nil {
			resp.CacheStatus = "unhealthy: Cache not initialized"
			resp.Status = "degraded"
		} else {
			resp.CacheStatus = "healthy"
		}

		log.Printf("[Health Check] Hit received from IP: %s", resp.ClientIP)

		statusCode := http.StatusOK
		if resp.Status != "ok" {
			statusCode = http.StatusServiceUnavailable
		}

		c.JSON(statusCode, resp)
	}
}
