package health

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/queue"
)

type HealthCheckResponse struct {
	Status         string `json:"status"`
	TechReadDB     string `json:"tech_read_db"`
	TechWriteDB    string `json:"tech_write_db"`
	CacheStatus    string `json:"cache_status,omitempty"`
	RedisStatus    string `json:"redis_status,omitempty"`
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

		// Check TechReadDB
		if err := database.PingTechReadDB(); err != nil {
			resp.TechReadDB = "unhealthy: " + err.Error()
			resp.Status = "degraded"
		} else {
			resp.TechReadDB = "healthy"
		}

		// Check TechWriteDB
		if err := database.PingTechWriteDB(); err != nil {
			resp.TechWriteDB = "unhealthy: " + err.Error()
			resp.Status = "degraded"
		} else {
			resp.TechWriteDB = "healthy"
		}

		// Check Redis client
		redis, _ := redis.GetRedisClient(config.Configs.RedisAddress, config.Configs.RedisPassword)

		// Ping Redis to check if the connection is alive
		if err := redis.Ping(context.Background()).Err(); err != nil {
			resp.RedisStatus = "unhealthy: " + err.Error()
			resp.Status = "degraded"
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

		statusCode := http.StatusOK
		if resp.Status != "ok" {
			statusCode = http.StatusServiceUnavailable
		}

		c.JSON(statusCode, resp)
	}
}
