package redisModels

// MobileChannelData represents the data structure stored in Redis for a mobile_channel key
type MobileChannelRedisData struct {
	TransactionId string `json:"transactionId,omitempty"`
	ErrorMessage  string `json:"errorMessage,omitempty"`
	ClaimedAtUnix int64  `json:"claimedAtUnix,omitempty"`
	AttemptCount  int    `json:"attemptCount,omitempty"`
	ClaimID       string `json:"claimId,omitempty"`
	Outcome       string `json:"outcome,omitempty"`
}
