//go:build generate

package main

import (
	"log"

	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/schema"
)

func main() {
	if err := schema.GenerateConfigSchemas(&config.Config{}); err != nil {
		log.Fatal(err)
	}
}