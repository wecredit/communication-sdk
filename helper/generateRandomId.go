package helper

import (
	"math/rand"
	"time"
)

func GenerateRandomID(from int, to int) int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return from + int(r.Int63n(int64(to-from+1)))
}
