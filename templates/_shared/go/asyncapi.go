// Command asyncapi prints the AsyncAPI document, for npm --prefix web run gen.
package main

import (
	"log"
	"os"

	"go.chanx.dev/v2/asyncapi"

	"app/internal/app"
)

func main() {
	doc, err := asyncapi.JSON(app.New(), app.Info)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(doc)
}
