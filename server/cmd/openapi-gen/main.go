// openapi-gen generates the OpenAPI (Swagger) YAML for the server's public API using reflection over the router.
// Run from server directory: go run ./cmd/openapi-gen
// Output is written to server/openapi.yaml (or openapi.yaml when run from server).
package main

import (
	"log"
	"path/filepath"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/openapi"
)

func main() {
	// When run from server/, write openapi.yaml in current dir
	outPath := "openapi.yaml"
	if wd, err := filepath.Abs("."); err == nil {
		outPath = filepath.Join(wd, "openapi.yaml")
	}
	if err := openapi.Generate(outPath); err != nil {
		log.Fatalf("generate openapi: %v", err)
	}
	log.Printf("wrote %s", outPath)
}
