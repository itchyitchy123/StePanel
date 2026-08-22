package main

import (
	"html/template"
	"log"
	"net/http"
)

type Server struct {
	Name   string
	Region string
	Status string
	Load   string
	Uptime string
}

type PageData struct {
	Title   string
	Servers []Server
}

func main() {
	tmpl := template.Must(template.ParseFiles("web/index.html"))
	data := PageData{
		Title: "StePanel",
		Servers: []Server{
			{Name: "edge-north-01", Region: "Helsinki · FI", Status: "Operational", Load: "18%", Uptime: "42d 08h"},
			{Name: "vault-core-02", Region: "Amsterdam · NL", Status: "Operational", Load: "31%", Uptime: "17d 03h"},
			{Name: "build-west-01", Region: "London · UK", Status: "Maintenance", Load: "—", Uptime: "—"},
		},
	}

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("render dashboard: %v", err)
		}
	})

	log.Println("StePanel listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
