package main

import (
	log "github.com/sirupsen/logrus"
	"strings"
	"unicode"
)

const OLLAMA_API_URL = "http://localhost:11434"

func main() {
	log.SetLevel(log.DebugLevel)
	err := MakeConverterService()
	if err != nil {
		panic(err)
	}
}

func MinimizeString(s string) string {
	var builder strings.Builder
	for _, r := range s {
		if !unicode.IsSpace(r) && !unicode.IsControl(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
