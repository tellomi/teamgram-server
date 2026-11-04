package tellomiai

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func getenv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func envOn(key string) bool {
	return os.Getenv(key) == "1"
}

func envInt(key string, def int) int {
	v := getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func envFloat(key string) float64 {
	v := getenv(key)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func envDurationSeconds(key string, def time.Duration) time.Duration {
	n := envInt(key, 0)
	if n <= 0 {
		return def
	}
	return time.Duration(n) * time.Second
}
