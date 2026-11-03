package discoverdelete

import "os"

func realGetenv(key string) string {
	return os.Getenv(key)
}
