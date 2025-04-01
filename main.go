package main

import (
	"job-scraper/config"
	"job-scraper/routes"
	"job-scraper/scraper"
	"log"
	"net/http"
)

func main() {
	config.ConnectDB()

	go scraper.StartScraper()         // Expatriates scraper
	go scraper.StartLinkedInScraper() // LinkedIn scraper

	router := routes.SetupRouter()

	log.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}
