package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

var (
	appLogger *log.Logger
	logFile   *os.File
	loggerMu  sync.Mutex
)

func initLogger() error {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	if appLogger != nil {
		return nil
	}

	file, err := os.OpenFile("central-sync.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	logFile = file

	writer := io.MultiWriter(os.Stdout, logFile)
	appLogger = log.New(writer, "", log.Ldate|log.Ltime)

	return nil
}

func closeLogger() error {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	if logFile == nil {
		return nil
	}

	err := logFile.Close()
	logFile = nil
	appLogger = nil
	return err
}

func logInfo(format string, args ...interface{}) {
	if appLogger == nil {
		fmt.Printf("INFO  "+format+"\n", args...)
		return
	}
	appLogger.Printf("INFO  "+format, args...)
}

func logWarn(format string, args ...interface{}) {
	if appLogger == nil {
		fmt.Printf("WARN  "+format+"\n", args...)
		return
	}
	appLogger.Printf("WARN  "+format, args...)
}

func logError(format string, args ...interface{}) {
	if appLogger == nil {
		fmt.Printf("ERROR "+format+"\n", args...)
		return
	}
	appLogger.Printf("ERROR "+format, args...)
}