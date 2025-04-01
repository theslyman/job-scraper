package routes

import (
	"encoding/json"
	"html/template"
	"job-scraper/config"
	"job-scraper/models"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func SetupRouter() *mux.Router {
	r := mux.NewRouter()
	// Serve static files (CSS, etc.)
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	r.HandleFunc("/", HomeHandler).Methods("GET")
	r.HandleFunc("/api/jobs", APIHandler).Methods("GET")
	return r
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	var jobs []models.Job
	sourceFilter := r.URL.Query().Get("source")
	query := config.DB
	if sourceFilter != "" {
		query = query.Where("source LIKE ?", "%" + sourceFilter + "%")
	}
	query.Find(&jobs)

	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return "Unknown"
			}
			return t.Format("02 Jan 2006 15:04")
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
	}
	tmpl := template.Must(template.New("index.html").Funcs(funcMap).ParseFiles("templates/index.html"))
	tmpl.Execute(w, jobs)
}


func APIHandler(w http.ResponseWriter, r *http.Request) {
	var jobs []models.Job
	config.DB.Find(&jobs)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}
