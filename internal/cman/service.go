package cman

import (
	"context"
	"fmt"

	"github.com/bhmj/goblocks/appstatus"
	"github.com/bhmj/goblocks/log"
	"github.com/bhmj/goblocks/metrics"

	"github.com/bhmj/cman/internal/cman/sandbox"
)

type Service struct {
	cfg            *Config
	logger         log.MetaLogger
	statusReporter appstatus.ServiceStatusReporter
	sandbox        sandbox.SandboxManager
}

// New returns cman service instance
func New(
	cfg *Config,
	logger log.MetaLogger,
	_ *metrics.Registry,
	statusReporter appstatus.ServiceStatusReporter,
	cfgPath string,
) (*Service, error) {
	sandbox, err := sandbox.New(logger, cfg.Sandbox, cfgPath)
	if err != nil {
		return nil, fmt.Errorf("create sandbox manager: %w", err)
	}

	svc := &Service{
		logger:         logger,
		cfg:            cfg,
		statusReporter: statusReporter,
		sandbox:        sandbox,
	}

	return svc, nil
}

func (s *Service) Run(ctx context.Context) error {
	s.statusReporter.Ready()
	<-ctx.Done()
	return nil
}

func (s *Service) GetSessionData(SID string) (any, error) {
	return nil, nil
}
