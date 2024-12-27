package main

import (
	"log"

	"fyne.io/fyne/v2/app"
)

func main() {
	// Inițializare aplicație
	a := app.New()
	w := createMainWindow(a)

	// Log pentru pornire
	log.Println("NFTClient started...")

	// Afișare fereastră principală
	w.ShowAndRun()
}
