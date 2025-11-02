package cman

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/bhmj/cman/internal/cman/sandbox"
)

// runRequest represents incoming execution request
type runRequest struct {
	Lang    string `json:"lang"`
	Version string `json:"version"`
	// cargo
	Files     map[string]string `json:"files"`
	MainFile  string            `json:"main"`
	StdinFile string            `json:"stdin"`
	// runtime limits
	RAM     uint `json:"ram"`     // Mb
	CPUs    uint `json:"cpus"`    // 1/1000 CPU
	CPUTime uint `json:"cputime"` // ms
	Net     uint `json:"net"`     // bytes
	RunTime uint `json:"runtime"` // sec
}

// RunHandler reads the request params and runs the container(s).
func (s *Service) RunHandler(w http.ResponseWriter, r *http.Request) (int, error) {
	var req runRequest
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&req)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("bad request: %w", err)
	}

	// prepare structs
	cargo := &sandbox.Cargo{
		Files:     req.Files,
		MainFile:  req.MainFile,
		StdinFile: req.StdinFile,
	}
	resources := &sandbox.Resource{ //nolint:exhaustruct
		RAM:     req.RAM,
		CPUs:    req.CPUs,
		CPUTime: req.CPUTime,
		Net:     req.Net,
		RunTime: req.RunTime,
	}
	// run sandbox
	err = s.sandbox.CheckSupport(req.Lang, req.Version)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("check support: %w", err)
	}
	var wg sync.WaitGroup
	err = s.sandbox.Run(req.Lang, req.Version, cargo, resources, s.StreamWriter(w, &wg))
	if err != nil {
		wg.Wait()
		return http.StatusInternalServerError, fmt.Errorf("sandbox run: %w", err)
	}

	wg.Wait()
	return http.StatusOK, nil
}

func (s *Service) StreamWriter(w http.ResponseWriter, wg *sync.WaitGroup) sandbox.Streamer {
	replacer := strings.NewReplacer(`\`, `\\`, "\r", `\r`, "\n", `\n`, `:`, `\:`) // must escape ':' since it may appear at start and be mistreated.
	return func(stream chan sandbox.Datagram) {
		wg.Add(1)
		defer wg.Done()
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Default to SSE stream
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush() // flush headers
		for ev := range stream {
			fmt.Fprintf(w, "event: %s\n", ev.Event)
			fmt.Fprintf(w, "data: %s\n\n", replacer.Replace(string(ev.Data)))
			flusher.Flush() // flush chunk
		}
	}
}

func (s *Service) StatsHandler(w http.ResponseWriter, _ *http.Request) (int, error) {
	stats := s.sandbox.Stats()
	if stats == nil {
		return http.StatusInternalServerError, errors.New("failed to retrieve sandbox stats")
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	str := "Container manager:\n"
	for k, v := range stats.CmanContainers {
		str += fmt.Sprintf("  %s:\t%v\n", k[:12], v)
	}
	str += "\nSandbox manager:\n"
	for _, c := range stats.SandboxContainers {
		str += fmt.Sprintf("  %v\n", c.ContainerID[:12])
		str += fmt.Sprintf("    ID:\t\t%v\n", c.ID)
		str += fmt.Sprintf("    Lang:\t%s\n", c.Lang)
		str += fmt.Sprintf("    Version:\t%v\n", c.Version)
		str += fmt.Sprintf("    LastUsed:\t%v\n", c.LastUsed)
		str += fmt.Sprintf("    Resident:\t%v\n", c.Resident)
		str += fmt.Sprintf("    Rank:\t%v\n", c.Rank)
		str += fmt.Sprintf("    Busy:\t%v\n", c.Busy)
	}
	_, _ = w.Write([]byte(str))
	return http.StatusOK, nil
}
