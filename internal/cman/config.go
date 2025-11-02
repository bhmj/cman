package cman

import (
	"github.com/bhmj/cman/internal/cman/sandbox"
)

// Config contains all parameters
type Config struct {
	APIBase string         `yaml:"api_base" default:"api/" description:"API base"`
	Sandbox sandbox.Config `yaml:"sandbox" group:"Sandbox settings"`
}
