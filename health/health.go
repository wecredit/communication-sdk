package health

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/queue"
)

// Health status constants
const (
	StatusOK        = "ok"
	StatusDegraded  = "degraded"
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"
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

// healthCheckResult represents the result of a single health check
type healthCheckResult struct {
	status    string
	isHealthy bool
}

// checkHealth performs a health check and returns the result
func checkHealth(checkFunc func() error, healthyMsg string) healthCheckResult {
	if err := checkFunc(); err != nil {
		return healthCheckResult{
			status:    fmt.Sprintf("%s: %v", StatusUnhealthy, err),
			isHealthy: false,
		}
	}
	return healthCheckResult{
		status:    healthyMsg,
		isHealthy: true,
	}
}

// checkRedisHealth performs Redis health check with proper error handling
func checkRedisHealth() healthCheckResult {
	r, err := redis.GetRedisClient(config.Configs.RedisAddress, config.Configs.RedisPassword)
	if err != nil {
		return healthCheckResult{
			status:    fmt.Sprintf("%s: Failed to get Redis client: %v", StatusUnhealthy, err),
			isHealthy: false,
		}
	}

	if err := r.Ping(context.Background()).Err(); err != nil {
		return healthCheckResult{
			status:    fmt.Sprintf("%s: %v", StatusUnhealthy, err),
			isHealthy: false,
		}
	}

	return healthCheckResult{
		status:    StatusHealthy,
		isHealthy: true,
	}
}

// checkAWSQueueHealth checks if AWS queue clients are initialized
func checkAWSQueueHealth() healthCheckResult {
	if queue.SQSClient == nil || queue.SNSClient == nil {
		return healthCheckResult{
			status:    fmt.Sprintf("%s: AWS clients not initialized", StatusUnhealthy),
			isHealthy: false,
		}
	}
	return healthCheckResult{
		status:    StatusHealthy,
		isHealthy: true,
	}
}

// checkCacheHealth checks if cache is initialized
func checkCacheHealth() healthCheckResult {
	if cache.GetCache() == nil {
		return healthCheckResult{
			status:    fmt.Sprintf("%s: Cache not initialized", StatusUnhealthy),
			isHealthy: false,
		}
	}
	return healthCheckResult{
		status:    StatusHealthy,
		isHealthy: true,
	}
}

// HealthCheckHandler handles health checks using Gin
func HealthCheckHandler(port string) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := HealthCheckResponse{
			Status:     StatusOK,
		}

		// Perform all health checks
		techReadResult := checkHealth(database.PingTechReadDB, StatusHealthy)
		resp.TechReadDB = techReadResult.status

		techWriteResult := checkHealth(database.PingTechWriteDB, StatusHealthy)
		resp.TechWriteDB = techWriteResult.status

		redisResult := checkRedisHealth()
		resp.RedisStatus = redisResult.status

		awsQueueResult := checkAWSQueueHealth()
		resp.AWSQueueClient = awsQueueResult.status

		cacheResult := checkCacheHealth()
		resp.CacheStatus = cacheResult.status

		// Determine overall status
		if !techReadResult.isHealthy || !techWriteResult.isHealthy ||
			!redisResult.isHealthy || !awsQueueResult.isHealthy || !cacheResult.isHealthy {
			resp.Status = StatusDegraded
		}

		// Set appropriate HTTP status code
		statusCode := http.StatusOK
		if resp.Status != StatusOK {
			statusCode = http.StatusServiceUnavailable
		}

		c.JSON(statusCode, resp)
	}
}
