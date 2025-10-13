package redisModels

// MobileChannelData represents the data structure stored in Redis for a mobile_channel key
type MobileChannelRedisData struct {
	TransactionId string `json:"transactionId,omitempty"`
	ErrorMessage  string `json:"errorMessage,omitempty"`
}
