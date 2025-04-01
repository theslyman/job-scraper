package scraper

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"job-scraper/config"
	"job-scraper/models"

	"github.com/gocolly/colly"
)

var (
	scrapedCountExpatriates int  = 0
	stopExpatriates         bool = false
)

// StartScraper begins scraping jobs from expatriates.com with stopping conditions.
func StartScraper() {
	// Reset state for each run.
	scrapedCountExpatriates = 0
	stopExpatriates = false

	mainURL := "https://www.expatriates.com/classifieds/bahrain/jobs/"

	// Create the main collector.
	c := colly.NewCollector(
		colly.AllowedDomains("www.expatriates.com", "expatriates.com"),
		colly.UserAgent("Mozilla/5.0 (compatible; JobScraper/1.0)"),
	)

	// Clone a collector for job detail pages.
	detailCollector := c.Clone()

	// On the main page, find job links starting with "/cls/"
	c.OnHTML("a[href^='/cls/']", func(e *colly.HTMLElement) {
		link := e.Request.AbsoluteURL(e.Attr("href"))
		log.Println("Found job link:", link)
		detailCollector.Visit(link)
	})

	// Follow pagination links only if we haven't hit a stop condition.
	c.OnHTML("nav.pagination a", func(e *colly.HTMLElement) {
		if stopExpatriates {
			log.Println("Stop condition met for expatriates; not following further pagination links.")
			return
		}
		link := e.Request.AbsoluteURL(e.Attr("href"))
		log.Println("Following pagination link:", link)
		e.Request.Visit(link)
	})

	// Process each job detail page.
	detailCollector.OnHTML("html", func(e *colly.HTMLElement) {
		job := models.Job{}
		job.Source = e.Request.URL.String()

		pageHTML, err := e.DOM.Html()
		if err != nil {
			log.Println("Error getting HTML:", err)
			return
		}

		job.Title = strings.TrimSpace(extractBetween(pageHTML, "<!-- begin_title -->", "<!-- end_title -->"))
		job.Description = strings.TrimSpace(extractBetween(pageHTML, "<!-- begin_description -->", "<!--"))

		if emailHref, exists := e.DOM.Find("a[href^='mailto:']").First().Attr("href"); exists {
			job.Email = strings.TrimPrefix(emailHref, "mailto:")
		}

		if phoneHref, exists := e.DOM.Find("a[href^='tel:']").First().Attr("href"); exists {
			job.Phone = strings.TrimPrefix(phoneHref, "tel:")
		}

		if epochStr, exists := e.DOM.Find("span#timestamp").Attr("epoch"); exists {
			if epochInt, err := strconv.ParseInt(epochStr, 10, 64); err == nil {
				job.DatePublished = time.Unix(epochInt, 0)
			} else {
				log.Println("Error parsing epoch:", err)
			}
		}
		if job.DatePublished.IsZero() {
			dateText := strings.TrimSpace(e.DOM.Find("span#timestamp").Text())
			if dateText != "" {
				layout := "Monday, Jan 2, 2006, 3:04:05 PM"
				parsedDate, err := time.Parse(layout, dateText)
				if err != nil {
					log.Println("Error parsing date:", err, "DateText:", dateText)
				} else {
					job.DatePublished = parsedDate
				}
			}
		}

		job.Type = strings.TrimSpace(e.DOM.Find("strong:contains('Category:')").Parent().Text())

		{
			reqRe := regexp.MustCompile(`(?i)requirements\s*([^<\n]+)`)
			if reqMatch := reqRe.FindStringSubmatch(pageHTML); len(reqMatch) > 1 {
				job.Requirements = strings.TrimSpace(reqMatch[1])
			}
		}

		// Stop pagination if job is older than 3 days.
		if !job.DatePublished.IsZero() && time.Since(job.DatePublished) > 72*time.Hour {
			log.Println("Encountered job older than 3 days:", job.Title)
			stopExpatriates = true
			return
		}

		if job.Title != "" {
			if err := config.DB.Create(&job).Error; err != nil {
				log.Println("Error saving job:", err)
			} else {
				log.Println("Job saved:", job.Title)
				scrapedCountExpatriates++
			}
		} else {
			log.Println("Job title empty, skipping this posting")
		}

		if scrapedCountExpatriates >= 50 {
			log.Println("Reached 50 jobs for expatriates; stopping further scraping.")
			stopExpatriates = true
		}
	})

	if err := c.Visit(mainURL); err != nil {
		log.Println("Error visiting main page:", err)
	}

	// Re-run periodically.
	ticker := time.NewTicker(6 * time.Hour)
	for range ticker.C {
		scrapedCountExpatriates = 0
		stopExpatriates = false
		c.Visit(mainURL)
	}
}

// extractBetween returns the substring between the start and end markers.
func extractBetween(s, start, end string) string {
	i := strings.Index(s, start)
	if i == -1 {
		return ""
	}
	i += len(start)
	j := strings.Index(s[i:], end)
	if j == -1 {
		return s[i:]
	}
	return s[i : i+j]
}
