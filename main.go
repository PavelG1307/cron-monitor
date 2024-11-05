package main

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type CronJob struct {
	Script   string `json:"script"`
	Hash     string `json:"hash"`
	Interval string `json:"interval"`
}

var hashToScriptMap = map[string]string{}

func makeMD5(value string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(value)))
}

func getCrontabRawString() (string, error) {
	cmd := exec.Command("crontab", "-l")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return out.String(), nil
}

func parseCronJob(raw string) *CronJob {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") {
		return nil
	}

	parts := strings.Fields(raw)
	if len(parts) < 6 {
		return nil
	}

	interval := strings.Join(parts[:5], " ")
	script := strings.Join(parts[5:], " ")
	hash := makeMD5(script)

	return &CronJob{
		Script:   script,
		Interval: interval,
		Hash:     hash,
	}
}
func getCronJobsFromSystem() ([]CronJob, error) {
	rawString, err := getCrontabRawString()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(rawString, "\n")

	var cronJobs []CronJob

	for _, line := range lines {
		cronJob := parseCronJob(line)
		if cronJob == nil {
			continue
		}

		cronJobs = append(cronJobs, *cronJob)

		hashToScriptMap[cronJob.Hash] = cronJob.Script
	}

	return cronJobs, nil
}

func getCronJobsHandler(w http.ResponseWriter, r *http.Request) {
	cronJobs, err := getCronJobsFromSystem()
	if err != nil {
		http.Error(w, "Failed to retrieve cron jobs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(cronJobs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func startCronJobHandler(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	script, ok := hashToScriptMap[hash]
	if !ok {
		http.Error(w, "Job not found", http.StatusBadRequest)
	}

	cmd := exec.Command("sh", "-c", script)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.Write(out.Bytes())
}

func main() {

	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	router.Get("/cronjobs", getCronJobsHandler)
	router.Post("/cronjobs/{hash}", startCronJobHandler)

	httpAddress := net.JoinHostPort("", fmt.Sprint(8080))

	log.Printf("Starting server on %s\n", httpAddress)

	if err := http.ListenAndServe(httpAddress, router); err != nil {
		log.Fatalf("Server failed: %s", err)
	}
}
