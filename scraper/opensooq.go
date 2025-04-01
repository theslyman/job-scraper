package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"job-scraper/config" // Adjust to your project's config package path
	"job-scraper/models" // Adjust to your project's models package path
)

// Constants for OpenSooq scraping
const (
	openSooqJobsURL = "https://bh.opensooq.com/en/jobs/job-vacancies"
	maxJobs         = 150 // Maximum new jobs to scrape
)

// HTTP headers for OpenSooq requests
var openSooqHeaders = map[string]string{
	"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
	"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
	"Accept-Language": "en-US,en;q=0.5",
}

// StartOpenSooqScraper initializes the OpenSooq scraper and runs it periodically.
func StartOpenSooqScraper() {
	log.Println("Starting OpenSooq scraper")

	// Set up context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Handle graceful shutdown on interrupt signal (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received interrupt signal, shutting down gracefully...")
		cancel()
		os.Exit(0)
	}()

	// Run the scraper immediately
	scrapeOpenSooqJobs(ctx)

	// Schedule it to run every 6 hours
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			scrapeOpenSooqJobs(ctx)
		case <-ctx.Done():
			log.Println("Scraper context canceled, exiting loop")
			return
		}
	}
}

// scrapeOpenSooqJobs performs the scraping of OpenSooq job listings.
func scrapeOpenSooqJobs(ctx context.Context) {
	var scrapedCount int
	client := &http.Client{}

	log.Println("Fetching OpenSooq jobs from:", openSooqJobsURL)
	html, err := fetchOpenSooqPage(client)
	if err != nil {
		log.Println("Error fetching OpenSooq page:", err)
		return
	}

	jobs, err := extractOpenSooqJobs(html)
	if err != nil {
		log.Println("Error extracting jobs:", err)
		return
	}
	log.Println("Found", len(jobs), "job listings")

	// Process each job
	seenURLs := make(map[string]bool)
	for _, job := range jobs {
		if scrapedCount >= maxJobs {
			log.Println("Reached maximum job limit:", maxJobs)
			break
		}
		if _, exists := seenURLs[job.Source]; exists {
			continue
		}
		seenURLs[job.Source] = true

		// Check for duplicates
		var count int64
		err = config.DB.Model(&models.Job{}).Where("source = ?", job.Source).Count(&count).Error
		if err != nil {
			log.Println("Error checking duplicate for", job.Title, ":", err)
			continue
		}
		if count > 0 {
			log.Println("Skipping duplicate job:", job.Title)
			continue
		}

		// Save to database
		job.DateScraped = time.Now()
		err = config.DB.Create(&job).Error
		if err != nil {
			log.Println("Error saving job", job.Title, ":", err)
		} else {
			log.Println("Saved job:", job.Title)
			scrapedCount++
		}
	}
	log.Println("Scraping complete. Total new jobs saved:", scrapedCount)
}

// fetchOpenSooqPage retrieves the HTML content of the OpenSooq jobs page.
func fetchOpenSooqPage(client *http.Client) (string, error) {
	req, err := http.NewRequest("GET", openSooqJobsURL, nil)
	if err != nil {
		return "", err
	}
	for k, v := range openSooqHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// extractOpenSooqJobs extracts job listings from the HTML content.
func extractOpenSooqJobs(html string) ([]models.Job, error) {
	// Find JSON-LD script tags using regex
	re := regexp.MustCompile(`<script type="application/ld\+json">(.*?)</script>`)
	matches := re.FindAllStringSubmatch(html, -1)

	var jobs []models.Job
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		jsonStr := match[1]

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue // Skip invalid JSON
		}

		if data["@type"] == "ItemList" {
			itemList, ok := data["itemListElement"].([]interface{})
			if !ok {
				continue
			}
			for _, item := range itemList {
				jobData, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				job := parseOpenSooqJob(jobData)
				if job != nil {
					jobs = append(jobs, *job)
				}
			}
		}
	}

	return jobs, nil
}

// parseOpenSooqJob converts JSON-LD job data into a models.Job struct.
func parseOpenSooqJob(jobData map[string]interface{}) *models.Job {
	title, _ := jobData["name"].(string)
	description, _ := jobData["description"].(string)
	url, _ := jobData["url"].(string)

	if title == "" || url == "" {
		return nil // Skip incomplete entries
	}

	// Parse date from description or use current time as fallback
	datePublished := time.Now()
	reDate := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`)
	if match := reDate.FindStringSubmatch(description); len(match) > 1 {
		if parsedDate, err := time.Parse("2006-01-02", match[1]); err == nil {
			datePublished = parsedDate
		}
	}

	return &models.Job{
		Title:         title,
		Description:   strings.TrimSpace(description),
		Source:        url,
		DatePublished: datePublished,
	}
}
