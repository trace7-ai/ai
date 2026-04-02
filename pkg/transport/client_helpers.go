package transport

import (
	"io"
	"os"
	"runtime"
	"time"
)

func ioReadAll(reader io.Reader) ([]byte, error) {
	return io.ReadAll(reader)
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func runtimeGOOS() string {
	return runtime.GOOS
}

func timeNow() string {
	return time.Now().Format("2006-01-02 15:04")
}
