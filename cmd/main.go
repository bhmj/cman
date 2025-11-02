package main

import (
	"fmt"
	syslog "log"

	"github.com/bhmj/goblocks/app"

	"github.com/bhmj/cman/internal/cman"
)

var appVersion = "local" //nolint:gochecknoglobals

func CmanFactory(config any, options app.Options) (app.Service, error) {
	cfg := config.(*cman.Config) //nolint:forcetypeassert
	svc, err := cman.New(cfg, options.Logger, options.MetricsRegistry, options.ServiceReporter, options.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("create cman service: %w", err)
	}
	return svc, nil
}

func main() {
	app := app.New("container manager", appVersion)
	err := app.RegisterService("cman", &cman.Config{}, CmanFactory) //nolint:exhaustruct
	if err != nil {
		syslog.Fatalf("register service: %v", err)
	}
	app.Run(nil)
}
