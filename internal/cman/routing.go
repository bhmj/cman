package cman

import (
	"fmt"
	"strings"

	"github.com/bhmj/goblocks/app"
)

// GetHandlers returns a list of handlers for the server
func (s *Service) GetHandlers() []app.HandlerDefinition {
	apiBase := strings.Trim(s.cfg.APIBase, "/")
	api := func(path string) string {
		return fmt.Sprintf("/%s/%s", apiBase, strings.TrimPrefix(path, "/"))
	}
	return []app.HandlerDefinition{
		{Endpoint: "/run", Method: "POST", Path: api("/run/"), Func: s.RunHandler},
		{Endpoint: "/stats", Method: "GET", Path: "/stats/", Func: s.StatsHandler},
	}
}
