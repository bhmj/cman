package sandbox

import "time"

type sandboxCategory string

const (
	categoryCompiler    sandboxCategory = "compiler"
	categoryInterpreter sandboxCategory = "interpreter"
)

// Resource represent sandbox resources, configured or requested, depending on use context
type Resource struct {
	RAM     uint `yaml:"ram" description:"RAM, Mb" required:"true"`             // Mb
	CPUs    uint `yaml:"cpus" description:"CPUs, mCPU" required:"true"`         // mCPU
	CPUTime uint `yaml:"cpu_time" description:"CPU time, msec" required:"true"` // mCPU/sec
	Net     uint `yaml:"net" description:"Network traffic, bytes"`              // bytes
	RunTime uint `yaml:"run_time" description:"Run time, sec" required:"true"`  // sec
	TmpDir  uint `yaml:"tmp_dir" description:"/tmp/ dir size, Mb"`              // Mb
}

// Config defines service params
type Config struct {
	RootDir         string        `yaml:"root_dir" description:"Sandbox root dir, where all the mapping is done"`
	LimitTotalRAM   uint          `yaml:"limit_total_ram" description:"Total RAM limit, Mb" default:"1800"`
	RetentionPeriod time.Duration `yaml:"retention_period" description:"Time since last used when non-default container stopped" default:"15m"`
	Limits          Resource      `yaml:"limits"`
	Defaults        Resource      `yaml:"defaults"`
	Sandboxes       []string      `yaml:"sandboxes" long:"sandboxes" env:"SANDBOXES" description:"List of sandbox YAML files, absolute or relative to main config"`
	ShutdownDelay   time.Duration `yaml:"shutdown_delay" long:"shutdown-delay" env:"SHUTDOWN_DELAY" description:"Grace period from readiness off to server down" default:"6s"`
}

// SandboxConfig defines settings for specific programming language (in Versions and VersionScript the key is interface' language version!)
type SandboxConfig struct { //nolint:revive
	Language          string                    `yaml:"name" description:"Language name" required:"true"` // Go, Cpp, Python, etc. [A-Za-z] only!
	Category          sandboxCategory           `yaml:"category" description:"Language processing category" choice:"compiler,interpreter" required:"true"`
	Resident          bool                      `yaml:"resident" description:"This language group container should persist in memory"` // the latest ver only!
	DefaultCmd        bool                      `yaml:"default_cmd" description:"if true, then the default image's CMD is used, not CMAN's empty loop"`
	ReadyString       string                    `yaml:"ready_string" description:"if set, defines the substring to look for before the container is considered started"`
	ReadyTimeout      time.Duration             `yaml:"ready_timeout" description:"applicable if ReadyString is set, defines the timeout for grepping the ReadyString"`
	CompilerBaseImage string                    `yaml:"compiler_image" description:"Compiler docker image" required:"true"`
	RunnerBaseImage   string                    `yaml:"runner_image" description:"Runner docker image"` // runner is universal for all compiler versions!
	DefaultScript     string                    `yaml:"default_script" description:"Default run (compile, interpret) script"`
	Versions          map[string]SandboxVersion `yaml:"versions" description:"Image-specific parameters"`
	CompilerResources *Resource                 `yaml:"compiler_resources" description:"Compiler resources"`
	LatestVersions    versionList               // not configurable: latest among versions is stored here at run time
}

type SandboxVersion struct { //nolint:revive
	ImageTag     string        `yaml:"image_tag" description:"Docker image gat"`
	Script       string        `yaml:"script" description:"Version-specific run script"`
	CacheVolumes []CacheVolume `yaml:"cache_volumes" description:"Cache volumes (common for all versions)"`
}

type CacheVolume struct {
	VolumeName string `yaml:"volume_name" long:"volume-name" env:"VOLUME_NAME" description:"Volume name. Will be prefixed by language and postfixed by version"`
	MountPath  string `yaml:"mount_path" long:"mount-path" env:"MOUNT_PATH" description:"Mount path inside container"`
}

type versionList []string

func (v *versionList) includes(ver string) bool {
	for _, vv := range *v {
		if vv == ver {
			return true
		}
	}

	return false
}
