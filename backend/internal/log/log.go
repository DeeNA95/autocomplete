package log

import (
	"log"
	"os"
)

var (
	// InfoLogger for standard, non-error messages.
	InfoLogger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	// ErrorLogger for error messages.
	ErrorLogger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
)
