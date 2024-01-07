package gotemplates

import (
	"fmt"
	"os"
)

func GetEnv(key, val string) string {
	if v := os.Getenv(key); v == "" {
		fmt.Printf("no env found. use default env: %s=%s\n", key, val)
		return val
	} else {
		fmt.Printf("read env: %s=%s\n", key, val)
		return v
	}
}
