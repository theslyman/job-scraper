package scraper

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"job-scraper/config"
	"job-scraper/models"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

var (
	scrapedCountLinkedIn int  = 0
	stopLinkedIn         bool = false
)

// StartLinkedInScraper scrapes jobs from LinkedIn using chromedp for dynamic content.
func StartLinkedInScraper() {
	scrapedCountLinkedIn = 0
	stopLinkedIn = false

	linkedInURL := "https://www.linkedin.com/jobs/jobs-in-bahrain"

	// Set up chromedp with a user agent to mimic a real browser
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Navigate to LinkedIn and get the rendered HTML
	var listingHTML string
	err := chromedp.Run(ctx,
		chromedp.Navigate(linkedInURL),
		chromedp.WaitVisible(`.jobs-search__results-list`, chromedp.ByQuery), // Wait for job listings to load
		chromedp.OuterHTML("html", &listingHTML),
	)
	if err != nil {
		log.Println("Error navigating to LinkedIn:", err)
		return
	}

	// Parse the HTML with goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(listingHTML))
	if err != nil {
		log.Println("Error parsing listing HTML:", err)
		return
	}

	// Extract job links and process each one
	doc.Find("a.base-card__full-link").Each(func(i int, s *goquery.Selection) {
		if stopLinkedIn {
			return
		}
		link, exists := s.Attr("href")
		if !exists {
			return
		}
		link = strings.TrimSpace(link)
		log.Println("Found LinkedIn job link:", link)

		// Visit the job detail page
		var jobHTML string
		err = chromedp.Run(ctx,
			chromedp.Navigate(link),
			chromedp.WaitVisible(`h1.top-card-layout__title`, chromedp.ByQuery),
			chromedp.OuterHTML("html", &jobHTML),
		)
		if err != nil {
			log.Println("Error visiting job page:", err)
			return
		}

		// Parse job details
		jobDoc, err := goquery.NewDocumentFromReader(strings.NewReader(jobHTML))
		if err != nil {
			log.Println("Error parsing job HTML:", err)
			return
		}

		job := models.Job{}
		job.Source = link
		job.Title = strings.TrimSpace(jobDoc.Find("h1.top-card-layout__title").Text())
		job.Description = strings.TrimSpace(jobDoc.Find(".description__text").Text())
		job.Type = "LinkedIn"
		job.Email = "" // LinkedIn rarely shows emails
		job.Phone = "" // LinkedIn rarely shows phone numbers

		// Parse relative posting date (e.g., "3 days ago")
		relativeDate := strings.TrimSpace(jobDoc.Find("span.posted-time-ago__text").Text())
		job.DatePublished = parseRelativeDate(relativeDate)

		// Save jobs posted within the last 3 days
		if !job.DatePublished.IsZero() && time.Since(job.DatePublished) <= 72*time.Hour && job.Title != "" {
			if err := config.DB.Create(&job).Error; err != nil {
				log.Println("Error saving LinkedIn job:", err)
			} else {
				log.Println("LinkedIn job saved:", job.Title)
				scrapedCountLinkedIn++
			}
		}

		if scrapedCountLinkedIn >= 150 {
			log.Println("Reached 150 LinkedIn jobs; stopping.")
			stopLinkedIn = true
		}

		// Avoid rate limiting with a delay
		time.Sleep(2 * time.Second)
	})

	// Optional: Re-run every 6 hours
	ticker := time.NewTicker(6 * time.Hour)
	for range ticker.C {
		scrapedCountLinkedIn = 0
		stopLinkedIn = false
		// Repeat the scraping process
		chromedp.Run(ctx, chromedp.Navigate(linkedInURL) /* ... repeat logic ... */)
	}
}

// parseRelativeDate converts "X days ago" to a time.Time value
func parseRelativeDate(rel string) time.Time {
	now := time.Now()
	var days int
	if strings.Contains(rel, "day") {
		_, err := fmt.Sscanf(rel, "%d day", &days)
		if err == nil {
			return now.AddDate(0, 0, -days)
		}
	}
	return time.Time{}
}
