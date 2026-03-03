package sandbox

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	conf "github.com/bhmj/goblocks/conftool"
	"github.com/bhmj/goblocks/containermanager"
	"github.com/bhmj/goblocks/file"
	"github.com/bhmj/goblocks/log"
	"github.com/bhmj/goblocks/retry"
)

type (
	outputEvent string
)

const (
	EvContentType outputEvent = "content-type" // start of binary transfer
	EvBinary      outputEvent = "binary"       // interpreter/runner, typed binary data, base64
	EvExit1       outputEvent = "exit1"        // compiler/interpreter, exit event
	EvExit2       outputEvent = "exit2"        // runner, exit event
	EvDone        outputEvent = "done"         // compiler/runner, run finished
	EvConsumed    outputEvent = "consumed"     // consumed resources
)

var (
	errConfigLanguageMissing     = errors.New("language name missing")
	errConfigInvalidCategory     = errors.New("invalid category")
	errConfigCompilerBaseImage   = errors.New("compiler base image missing")
	errConfigNoVersions          = errors.New("no versions specified")
	errLanguageNotSupported      = errors.New("language not supported")
	errVersionNotSupported       = errors.New("version not supported")
	errCompilerResourcesRequired = errors.New("must specify compiler resources")
	// errNotEnoughMemory         = errors.New("not enough memory")
	// errSlotNotFound            = errors.New("slot not found")
)

type Datagram struct {
	Event outputEvent
	Data  []byte
}

// runningResources represent resources of a running container. Used for container selection/filter.
type runningResources struct {
	RAM    uint
	CPUs   uint
	Net    bool
	TmpDir uint
}

// Cargo describes a data for running inside the container
type Cargo struct {
	Files     map[string]string // files to create
	MainFile  string            // file from package to run
	StdinFile string            // file from package to pipe out
}

type SandboxStats struct { //nolint:revive
	SandboxContainers []ContainerInfo
	CmanContainers    map[string]string
}

type ContainerInfo struct {
	ID            uint64
	ContainerID   string
	Lang, Version string
	LastUsed      time.Time
	Resident      bool
	Rank          int // 0=busy, 1=resident, 2=unused
	Busy          bool
}

// Streamer reads data from [containerStream].
type Streamer func(stream chan Datagram)

type SandboxManager interface { //nolint:revive
	CheckSupport(lang, version string) error
	Run(lang, version string, cargo *Cargo, resources *Resource, streamer Streamer) error
	Stats() *SandboxStats
	Cleanup()
}

type sandboxManager struct {
	sync.RWMutex
	ContainerRunID atomic.Uint64
	ContainerUseID atomic.Uint64
	logger         log.MetaLogger
	cfg            Config
	cfgs           []SandboxConfig // simple list of configurations
	cman           containermanager.ContainerManager
	containers     []*runningContainer
	retrier        retry.Policy
}

type runningContainer struct {
	Lang        string
	Version     string
	ID          uint64
	UseID       uint64
	Busy        bool
	ContainerID string
	Script      string
	Resources   *runningResources
	Resident    bool
	Rank        int
	LastUsed    time.Time
}

func New(logger log.MetaLogger, cfg Config, cfgPath string) (SandboxManager, error) {
	cfgs, err := loadConfigs(cfg.Sandboxes, cfgPath)
	if err != nil {
		return nil, err
	}

	cman, err := containermanager.New(logger)
	if err != nil {
		return nil, fmt.Errorf("new containermanager: %w", err)
	}

	// cleanup stale containers
	running, _ := cman.FindContainers("^cman-")
	if len(running) > 0 {
		for _, containerID := range running {
			go cman.StopContainer(containerID, true)
		}
	}

	// list images
	err = listImages(cman, cfgs)
	if err != nil {
		return nil, err
	}

	// cleanup playground directory

	sm := &sandboxManager{ //nolint:exhaustruct
		cman:       cman,
		cfg:        cfg,
		cfgs:       cfgs,
		logger:     logger,
		containers: make([]*runningContainer, 0),
		retrier: retry.Policy{ //nolint:exhaustruct
			Backoff:     200 * time.Millisecond, //nolint:mnd
			Multiplier:  1,
			MaxAttempts: 5, //nolint:mnd
		},
	}

	go func() {
		t := time.NewTicker(time.Minute)
		for range t.C {
			sm.tidy()
		}
	}()

	return sm, nil
}

func (sm *sandboxManager) Stats() *SandboxStats {
	sm.RLock()
	defer sm.RUnlock()

	stats := &SandboxStats{
		CmanContainers:    sm.cman.Stats(),
		SandboxContainers: nil,
	}
	for _, container := range sm.containers {
		info := ContainerInfo{
			ID:          container.ID,
			ContainerID: container.ContainerID,
			Lang:        container.Lang,
			Version:     container.Version,
			LastUsed:    container.LastUsed,
			Resident:    container.Resident,
			Rank:        container.Rank,
			Busy:        container.Busy,
		}
		stats.SandboxContainers = append(stats.SandboxContainers, info)
	}

	return stats
}

func listImages(cman containermanager.ContainerManager, cfgs []SandboxConfig) error {
	check := make(map[string]bool)

	for _, cfg := range cfgs {
		for _, settings := range cfg.Versions {
			check[cfg.CompilerBaseImage+":"+settings.ImageTag] = false
		}

		if cfg.Category == categoryCompiler && !check[cfg.RunnerBaseImage] {
			check[cfg.RunnerBaseImage] = false
		}
	}

	for img := range check {
		if err := cman.ImageExist(img); err != nil {
			return fmt.Errorf("%s: %w", img, err)
		}
	}

	return nil
}

func (sm *sandboxManager) CheckSupport(lang, version string) error {
	lang = strings.ToLower(lang)
	version = strings.ToLower(version)
	_, err := sm.getSandboxConfig(lang, version)
	return err
}

func (sm *sandboxManager) Run(lang, version string, cargo *Cargo, requestedResource *Resource, streamer Streamer) error { //nolint:funlen
	defer sm.tidy()

	lang = strings.ToLower(lang)
	version = strings.ToLower(version)

	cfg, err := sm.getSandboxConfig(lang, version)
	if err != nil {
		return err
	}

	sm.sanityCheck(requestedResource)

	// upstream channels
	stream := make(chan Datagram)
	// downstream channels
	stdOutD := make(chan []byte)
	stdErrD := make(chan []byte)
	consumed := make(chan containermanager.ConsumedResources)
	// pipe is a combination of upstream/downstream channels
	pipe := containermanager.ContainerPipe{StdOut: stdOutD, StdErr: stdErrD, Consumed: consumed} //nolint:exhaustruct

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup

	go streamer(stream) // start the receiver

	stageSignal := make(chan int)
	go sm.streamProxy(ctx, &wg, stageSignal, stream, nil, stdOutD, stdErrD, consumed) // start the proxying streamer

	cleanup := func() {
		cancel()
		wg.Wait()
		close(stdOutD)
		close(stdErrD)
		close(consumed)
		close(stream)
	}
	defer cleanup()

	stage := 1 // compiler/interpreter

	var compiler *runningContainer

runStage1:
	for {
		containerUseID := sm.ContainerUseID.Add(1)

		res := sm.resourceSetup(requestedResource, cfg, stage)
		compiler, err = sm.findOrCreateContainer(cfg, stage, lang, version, res, containerUseID)
		if errors.Is(err, containermanager.ErrContainerCreate) ||
			errors.Is(err, containermanager.ErrContainerStart) ||
			errors.Is(err, containermanager.ErrContainerReady) {
			// container creation errors are fatal
			stdErrD <- []byte(err.Error())
			return err
		}

		defer func(contID string, useID uint64) { sm.setContainerIdle(contID, useID) }(compiler.ContainerID, containerUseID)

		err = sm.prepareSources(compiler, cargo)
		if err != nil {
			stdErrD <- []byte(err.Error())
			return err
		}

		// compiler/interpreter limits
		limits := containermanager.RuntimeLimits{
			CPUTime: res.CPUTime,
			Net:     res.Net,
			RunTime: res.RunTime,
			TmpDir:  res.TmpDir,
		}

		// run the compiler/interpreter
		command := []string{"sh", "-c", compiler.Script}
		code, err := sm.cman.Execute(compiler.ContainerID, command, pipe, limits)
		msg := "code:" + strconv.Itoa(code)
		if err != nil {
			msg += " error:" + err.Error()
			if errors.Is(err, containermanager.ErrContainerDoesNotExist) {
				sm.logger.Error("container does not exist", log.String("id", compiler.ContainerID[:8]))
				sm.unregisterContainer(compiler.ContainerID)
				continue runStage1
			}
		}
		stream <- Datagram{Event: EvExit1, Data: []byte(msg)}

		if code != 0 || err != nil {
			stream <- Datagram{Event: EvDone, Data: []byte("err")}
			return nil
		}

		if cfg.Category == categoryInterpreter {
			stream <- Datagram{Event: EvDone, Data: []byte("idone")}
			return nil
		}

		break
	}

	stage = 2
	stageSignal <- stage

	// find/create runner
	containerUseID := sm.ContainerUseID.Add(1)
	res := sm.resourceSetup(requestedResource, cfg, stage)
	runner, err := sm.findOrCreateContainer(cfg, stage, lang, version, res, containerUseID)
	if err != nil {
		return err
	}
	defer sm.setContainerIdle(runner.ContainerID, containerUseID)

	err = sm.moveBinary(compiler, runner)
	if err != nil {
		return err
	}

	sm.setContainerIdle(compiler.ContainerID, containerUseID) // release compiler as soon as we are done with it

	// runner limits
	limits := containermanager.RuntimeLimits{
		CPUTime: res.CPUTime,
		Net:     res.Net,
		RunTime: res.RunTime,
		TmpDir:  res.TmpDir,
	}

	// run the runner
	code, err := sm.cman.Execute(runner.ContainerID, []string{"./main"}, pipe, limits)
	msg := "code:" + strconv.Itoa(code)
	if err != nil {
		msg += " error:" + err.Error()
		if errors.Is(err, containermanager.ErrContainerDoesNotExist) {
			sm.unregisterContainer(runner.ContainerID)
		}
	}
	stream <- Datagram{Event: EvExit2, Data: []byte(msg)}
	stream <- Datagram{Event: EvDone, Data: []byte("xdone")}

	return nil
}

func (sm *sandboxManager) Cleanup() {
	sm.logger.Info("cleanup containers")
	wg := sync.WaitGroup{}

	for _, container := range sm.containers {
		sm.logger.Info("container", log.String("id", container.ContainerID[:8]), log.String("lang", container.Lang), log.String("ver", container.Version), log.Bool("busy", container.Busy))
		wg.Add(1)
		go func(cID string) {
			sm.stopRunningContainer(cID)
			wg.Done()
		}(container.ContainerID)
	}
	wg.Wait()
	sm.logger.Info("cleanup done")
}

// sanityCheck checks requested resources for sanity and modifies them accordingly.
func (sm *sandboxManager) sanityCheck(requested *Resource) {
	if requested.CPUTime > sm.cfg.Limits.CPUTime {
		requested.CPUTime = sm.cfg.Limits.CPUTime
	}
	if requested.CPUs > sm.cfg.Limits.CPUs {
		requested.CPUs = sm.cfg.Limits.CPUs
	}
	if requested.Net > sm.cfg.Limits.Net {
		requested.Net = sm.cfg.Limits.Net
	}
	if requested.RAM > sm.cfg.Limits.RAM {
		requested.RAM = sm.cfg.Limits.RAM
	}
	if requested.RunTime > sm.cfg.Limits.RunTime {
		requested.RunTime = sm.cfg.Limits.RunTime
	}
	if requested.TmpDir > sm.cfg.Limits.TmpDir {
		requested.TmpDir = sm.cfg.Limits.TmpDir
	}
}

// resourceSetup alters the requested resources depending on the stage (compiler have own resource config) and sets default vaules.
func (sm *sandboxManager) resourceSetup(requested *Resource, cfg *SandboxConfig, stage int) *Resource {
	result := *requested // copy

	if cfg.Category == categoryCompiler && stage == 1 { //nolint:nestif
		// compilers have their own resource config
		result = *cfg.CompilerResources
	} else {
		// defaults for omitted values
		if result.CPUTime == 0 {
			result.CPUTime = sm.cfg.Defaults.CPUTime
		}
		if result.CPUs == 0 {
			result.CPUs = sm.cfg.Defaults.CPUs
		}
		if result.RAM == 0 {
			result.RAM = sm.cfg.Defaults.RAM
		}
		if result.RunTime == 0 {
			result.RunTime = sm.cfg.Defaults.RunTime
		}
		// Net can be 0, no need to default
	}
	return &result
}

func (sm *sandboxManager) streamProxy(
	ctx context.Context,
	wg *sync.WaitGroup,
	stageSignal chan int,
	stream chan Datagram,
	_ /*stdIn*/, stdOut, stdErr chan []byte,
	consumed chan containermanager.ConsumedResources,
) {
	wg.Add(1)
	defer wg.Done()

	line := 0
	stage := 1
	b64 := false
	contentType := []byte("Content-Type:")
	for {
		select {
		case <-ctx.Done():
			return
		case st := <-stageSignal:
			stage = st
		case chunk := <-stdOut:
			// 1st line of stdout containing "Content-Type: something" starts "binary" streaming
			if !b64 && line == 0 && len(chunk) > len(contentType) && bytes.Equal(chunk[:len(contentType)], contentType) {
				b64 = true
				eol := 0
				for ; eol < len(chunk); eol++ {
					if chunk[eol] == '\n' {
						break
					}
				}
				stream <- Datagram{Event: EvContentType, Data: chunk[:eol]} // content-type without \n
				if eol < len(chunk) {
					chunk = chunk[eol+1:]
				} else {
					chunk = []byte{}
				}
			}
			if b64 {
				encoded := make([]byte, base64.StdEncoding.EncodedLen(len(chunk)))
				base64.StdEncoding.Encode(encoded, chunk)
				stream <- Datagram{Event: EvBinary, Data: encoded}
			} else {
				stream <- Datagram{Event: outputEvent("stdout" + strconv.Itoa(stage)), Data: chunk}
			}
			line++ //
		case chunk := <-stdErr:
			stream <- Datagram{Event: outputEvent("stderr" + strconv.Itoa(stage)), Data: chunk}
		case chunk := <-consumed:
			stream <- Datagram{Event: outputEvent(string(EvConsumed) + strconv.Itoa(stage)), Data: fmt.Appendf(nil, "%d,%d", chunk.CPUTime, chunk.Net)}
		}
	}
}

func (sm *sandboxManager) findOrCreateContainer(
	cfg *SandboxConfig,
	stage int,
	lang, version string,
	requested *Resource,
	useID uint64,
) (*runningContainer, error) {
	var result *runningContainer

	if stage == 2 { //nolint:mnd
		lang = "runner"
		version = ""
	}

	runningResources := &runningResources{
		RAM:    requested.RAM,
		CPUs:   requested.CPUs,
		Net:    requested.Net > 0,
		TmpDir: requested.TmpDir,
	}

	sm.Lock() // !!! access c.Busy
	for _, c := range sm.containers {
		if c.Lang == lang && c.Version == version && !c.Busy && *c.Resources == *runningResources {
			result = c
			result.Busy = true
			result.UseID = useID
			result.LastUsed = time.Now()
			if sm.cman.ContainerExist(c.ContainerID) { // NB: weak check
				sm.Unlock() // !!!
				return result, nil
			}
		}
	}
	sm.Unlock() // !!!

	// not found: create and run the container in idle state
	seqID := sm.ContainerRunID.Add(1)
	setup, script := sm.createContainerSetup(cfg, stage, seqID, lang, version, *requested)
	containerID, err := sm.cman.CreateAndRunContainer(setup)
	if err != nil {
		return nil, fmt.Errorf("create and run container: %w", err)
	}
	result = &runningContainer{
		Lang:        lang,
		Version:     version,
		ID:          seqID,
		UseID:       useID,
		Busy:        true,
		LastUsed:    time.Now(),
		ContainerID: containerID,
		Resources:   runningResources,
		Script:      script,
		Resident:    (cfg.Resident && cfg.LatestVersions.includes(version)) || stage == 2,
		Rank:        0,
	}
	sm.registerContainer(result)

	return result, nil
}

func (sm *sandboxManager) getSandboxConfig(lang, version string) (*SandboxConfig, error) {
	// find language
	iLang := -1
	for i, cfg := range sm.cfgs {
		if strings.ToLower(cfg.Language) == lang {
			iLang = i
			break
		}
	}
	if iLang < 0 {
		return nil, fmt.Errorf("%w: %s, %s", errLanguageNotSupported, lang, version)
	}
	// find version
	found := false
	cfg := sm.cfgs[iLang]
	for ver := range cfg.Versions {
		if strings.ToLower(ver) == version {
			found = true
			break
		}
	}
	if !found {
		return nil, errVersionNotSupported
	}
	return &cfg, nil
}

// createContainerSetup creates a ContainerSetup for containermanager, error if no matching config found
func (sm *sandboxManager) createContainerSetup(cfg *SandboxConfig, stage int, seqID uint64, lang, version string, resources Resource) (*containermanager.ContainerSetup, string) {
	var img string
	// Docker image
	switch stage {
	case 1:
		img = cfg.CompilerBaseImage + ":" + cfg.Versions[version].ImageTag
	case 2: //nolint:mnd
		img = cfg.RunnerBaseImage // runner is version independent
	}
	// label, volumes
	var label string
	var cacheVolume []string
	var cacheMount []string
	if stage == 1 {
		label = "cman-" + lang + "-" + version + "-" + strconv.FormatUint(seqID, 10)
		for i := range cfg.Versions[version].CacheVolumes {
			cacheVolume = append(cacheVolume, cfg.Versions[version].CacheVolumes[i].VolumeName)
			cacheMount = append(cacheMount, cfg.Versions[version].CacheVolumes[i].MountPath)
		}
	} else {
		// #nosec
		label = "cman-runner-" + strconv.FormatUint(seqID, 10)
		if resources.Net > 0 {
			label += "-w"
		}
	}
	setup := containermanager.ContainerSetup{
		Image:            img,
		DefaultCmd:       cfg.DefaultCmd,
		ReadyString:      cfg.ReadyString,
		ReadyTimeout:     cfg.ReadyTimeout,
		WorkingDir:       sm.getWorkingDir(seqID),
		WorkingDirRO:     cfg.Category != categoryCompiler || stage != 1, // writable only for compiler!
		CacheVolume:      cacheVolume,
		CacheVolumeMount: cacheMount,
		Label:            label,
		Resources: containermanager.Resources{
			RAM:    resources.RAM,
			CPUs:   resources.CPUs,
			Net:    resources.Net > 0,
			TmpDir: resources.TmpDir,
		},
	}
	// script
	script := cfg.DefaultScript
	specific := cfg.Versions[version].Script
	if specific != "" {
		script = specific
	}

	return &setup, script
}

// prepareSources copy files from cargo to working directory.
func (sm *sandboxManager) prepareSources(cont *runningContainer, cargo *Cargo) error {
	appDir := filepath.Join(sm.cfg.RootDir, fmt.Sprintf("%010d", cont.ID))
	err := file.Mkdir(appDir) // create if not exists
	if err != nil {
		return fmt.Errorf("create container dir: %w", err)
	}
	_ = file.ClearDirectory(appDir, true)
	// allow compiler to write to the container dir
	if err := os.Chmod(appDir, 0o777); err != nil { //nolint:mnd
		return fmt.Errorf("chmod container dir: %w", err)
	}
	for fname, content := range cargo.Files {
		// #nosec
		err = os.WriteFile(filepath.Join(appDir, fname), []byte(content), 0755) //nolint:mnd
		if err != nil {
			return fmt.Errorf("write %s to container dir: %w", fname, err)
		}
	}
	return nil
}

// moveBinary moves compiled binary to runner directory
func (sm *sandboxManager) moveBinary(from, to *runningContainer) error {
	fromPath := filepath.Join(sm.getWorkingDir(from.ID), "main")
	if !file.Exists(fromPath) {
		return errors.New("file " + fromPath + " not found!")
	}
	toPath := filepath.Join(sm.getWorkingDir(to.ID), "main")
	if file.Exists(toPath) {
		_ = file.Delete(toPath)
	}
	return file.Move(fromPath, toPath) //nolint:wrapcheck
}

// loadConfigs loads sandbox configs from [cfgPath] reading [configs] files.
func loadConfigs(configs []string, cfgPath string) ([]SandboxConfig, error) {
	cfgs := make([]SandboxConfig, 0)
	for _, path := range configs {
		filePath := filepath.Join(cfgPath, path)
		cfg := SandboxConfig{} //nolint:exhaustruct
		err := conf.ReadFromFile(filePath, &cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		// required fields check (TODO: use tags!)
		if cfg.Language == "" {
			return nil, fmt.Errorf("%s: %w", path, errConfigLanguageMissing)
		}
		if cfg.Category != categoryCompiler && cfg.Category != categoryInterpreter {
			return nil, fmt.Errorf("%s: %w", path, errConfigInvalidCategory)
		}
		if cfg.CompilerBaseImage == "" {
			return nil, fmt.Errorf("%s: %w", path, errConfigCompilerBaseImage)
		}
		if len(cfg.Versions) == 0 {
			return nil, fmt.Errorf("%s: %w", path, errConfigNoVersions)
		}
		hasRequiredFields := cfg.CompilerResources != nil && cfg.CompilerResources.RAM > 0 && cfg.CompilerResources.CPUTime > 0 && cfg.CompilerResources.CPUs > 0 && cfg.CompilerResources.RunTime > 0
		if cfg.Category == categoryCompiler && !hasRequiredFields {
			return nil, fmt.Errorf("%s: %w", path, errCompilerResourcesRequired)
		}
		cfg.LatestVersions = findLatest(cfg.Versions)
		cfgs = append(cfgs, cfg)
	}
	return cfgs, nil
}

type semVer struct {
	Source string
	Major  int
	Minor  int
	Patch  int
	Suffix string
}
type semVers []semVer

func (sv semVers) Len() int {
	return len(sv)
}
func (sv semVers) Less(i, j int) bool {
	acending := false // in reverse order!
	if sv[i].Major < sv[j].Major {
		return acending
	}
	if sv[i].Major > sv[j].Major {
		return !acending
	}
	if sv[i].Minor < sv[j].Minor {
		return acending
	}
	if sv[i].Minor > sv[j].Minor {
		return !acending
	}
	if sv[i].Patch < sv[j].Patch {
		return acending
	}
	if sv[i].Patch > sv[j].Patch {
		return !acending
	}
	if sv[i].Suffix < sv[j].Suffix {
		return acending
	}
	return !acending
}
func (sv semVers) Swap(i, j int) {
	sv[i], sv[j] = sv[j], sv[i]
}

func findLatest(versions map[string]SandboxVersion) []string {
	var vers semVers //nolint:prealloc
	for key := range versions {
		sv, err := parseSemVer(key)
		if err != nil {
			continue
		}
		vers = append(vers, sv)
	}
	sort.Sort(vers)
	var res []string
	for i := range vers {
		if vers[i].Major == vers[0].Major && vers[i].Minor == vers[0].Minor && vers[i].Patch == vers[0].Patch {
			res = append(res, vers[i].Source)
		}
	}
	return res
}

func parseSemVer(str string) (semVer, error) {
	// Define the regex for semantic versioning.
	re := regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([a-zA-Z0-9\.-]+))?$`)

	// Match the input string against the regex.
	matches := re.FindStringSubmatch(str)
	if matches == nil {
		return semVer{}, errors.New("invalid semantic version format")
	}

	// Convert the matched major, minor, and patch components to integers.
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	// Extract the suffix if it exists.
	suffix := matches[4]

	return semVer{
		Source: str,
		Major:  major,
		Minor:  minor,
		Patch:  patch,
		Suffix: suffix,
	}, nil
}

func (sm *sandboxManager) getWorkingDir(seqID uint64) string {
	return filepath.Join(sm.cfg.RootDir, fmt.Sprintf("%010d", seqID))
}

func (sm *sandboxManager) setContainerIdle(containerID string, useID uint64) {
	sm.Lock()
	defer sm.Unlock()
	for _, container := range sm.containers {
		if container.ContainerID == containerID && container.UseID == useID {
			container.Busy = false
		}
	}
}

func (sm *sandboxManager) tidy() {
	sm.Lock()
	defer sm.Unlock()

	// Rank: 0=busy, 1=resident, 2=unused

	// mark resident and unused containers
	for i, container := range sm.containers {
		// busy container is kept
		if container.Busy {
			container.Rank = 0
			continue
		}
		// non-resident container to be removed
		if !container.Resident {
			container.Rank = 2
			continue
		}
		// assume current container is to be kept
		container.Rank = 1
		// loop through already scanned (previous) containers and see if we already have container similar to current
		for p := range i {
			prev := sm.containers[p]
			if prev.Rank > 0 && container.Lang == prev.Lang && container.Version == prev.Version && *container.Resources == *prev.Resources {
				// found non-busy container similar to current: this one to be removed
				container.Rank = 2
				break
			}
		}
	}

	// unmark stale non-default resident containers
	for _, container := range sm.containers {
		if container.Rank == 1 && container.Lang == "runner" && time.Since(container.LastUsed) > sm.cfg.RetentionPeriod {
			// extra runners are to be removed
			defs := runningResources{
				RAM:    sm.cfg.Defaults.RAM,
				CPUs:   sm.cfg.Defaults.CPUs,
				Net:    sm.cfg.Defaults.Net > 0,
				TmpDir: sm.cfg.Defaults.TmpDir,
			}
			if *container.Resources != defs {
				// except a default runner
				container.Rank = 2
			}
		}
	}

	// remove unused containers
	for _, container := range sm.containers {
		if container.Rank > 1 {
			go sm.stopRunningContainer(container.ContainerID)
		}
	}
}

func (sm *sandboxManager) registerContainer(container *runningContainer) {
	sm.Lock()
	sm.containers = append(sm.containers, container)
	sm.Unlock()
}

func (sm *sandboxManager) unregisterContainer(containerID string) {
	sm.Lock()
	for i, c := range sm.containers {
		if c.ContainerID == containerID {
			sm.removeWorkingDir(sm.containers[i].ID)
			sm.containers = slices.Delete(sm.containers, i, i+1)
			break
		}
	}
	sm.Unlock()
}

func (sm *sandboxManager) removeWorkingDir(seqID uint64) {
	dir := sm.getWorkingDir(seqID)
	sm.logger.Info("removing dir", log.String("path", dir))
	_ = sm.retrier.Run(func(int) (error, error) {
		err := file.Rmdir(dir)
		if err != nil {
			sm.logger.Error("removing dir", log.Error(err))
		}
		return err, nil //nolint:wrapcheck
	})
}

func (sm *sandboxManager) stopRunningContainer(containerID string) {
	sm.logger.Info("stopping", log.String("container", containerID))
	sm.cman.StopContainer(containerID, false)
	sm.logger.Info("unregistering", log.String("container", containerID))
	sm.unregisterContainer(containerID)
}
