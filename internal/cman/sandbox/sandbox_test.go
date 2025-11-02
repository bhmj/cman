package sandbox

import (
	"os"
	"testing"

	"github.com/bhmj/goblocks/log"
)

const goConfigPath = "../../../config/go.yaml"

var (
	getStreamer func(*testing.T) func(chan Datagram)
	sm          SandboxManager
	cargo       *Cargo
	resources   *Resource
)

func TestMain(m *testing.M) {
	getStreamer = func(t *testing.T) func(chan Datagram) {
		return func(stream chan Datagram) {
			for ev := range stream {
				t.Logf("event: %v\n", ev.Event)
				t.Logf("data: %v\n", string(ev.Data))
			}
		}
	}

	resources = &Resource{
		RAM:     10240,
		CPUs:    1000,
		CPUTime: 10000,
		Net:     0,
		RunTime: 1000,
		TmpDir:  100,
	}

	wd, _ := os.Getwd()
	cfg := Config{
		RootDir:   wd,
		Sandboxes: []string{goConfigPath},
		Defaults:  *resources,
		Limits:    *resources,
	}
	logger, _ := log.New("error", false)
	sm, _ = New(logger, cfg, ".")
	cargo = &Cargo{
		Files:    map[string]string{"main.go": ""},
		MainFile: "main.go",
	}

	m.Run()
}

func TestBasic(t *testing.T) {
	cargo.Files["main.go"] = strBasic
	err := sm.Run("Go", "1.21", cargo, resources, getStreamer(t))
	if err != nil {
		t.Error(err)
	}
	sm.Cleanup()
}

func TestLong(t *testing.T) {
	cargo.Files["main.go"] = strLong
	err := sm.Run("Go", "1.21", cargo, resources, getStreamer(t))
	if err != nil {
		t.Error(err)
	}
	sm.Cleanup()
}

func TestCompileError(t *testing.T) {
	cargo.Files["main.go"] = strCompileError
	err := sm.Run("Go", "1.21", cargo, resources, getStreamer(t))
	if err != nil {
		t.Error(err)
	}
	sm.Cleanup()
}

func TestBinary(t *testing.T) {
	cargo.Files["main.go"] = strBinary
	err := sm.Run("Go", "1.21", cargo, resources, getStreamer(t))
	if err != nil {
		t.Error(err)
	}
	sm.Cleanup()
}

var strBasic string = `package main

import (
	"fmt"
	"time"
)

func wait(done chan struct{}) {
	time.Sleep(time.Second)
	done <- struct{}{}
}

func main() {
	done := make(chan struct{})
	fmt.Printf("init %v\n", time.Now())
	go wait(done)
	<-done
	fmt.Printf("done 1 %v\n", time.Now())
	go wait(done)
	<-done
	fmt.Printf("done 2 %v\n", time.Now())
}`

var strLong string = `package main

import (
	"fmt"
	"os"
	"encoding/binary"
)

var chunkSize = 120

func main() {
	fmt.Printf("chunk size: %d\n", chunkSize)
	fmt.Println("Contenz-Type: image/png")
	for i:=0; i<200; i++ {
		binary.Write(os.Stdout, binary.LittleEndian, getPNG(i))
	}
}

func getPNG(n int) []byte {
	str := fmt.Sprintf("%d:[", n)
	al := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i:=0; i<chunkSize; i++ {
		str += string(al[(i/2)%len(al)])
	}
	str += "]"
	return []byte(str)
}
`

var strCompileError string = `package main

import (
	"fmt"
)

func main() {
	fmt.Printf("unknown variable: %d", foo)
}
`

var strBinary string = `package main

import (
	"fmt"
	"os"
	"strings"
	"encoding/hex"
	"encoding/binary"
)

func main() {
	fmt.Printf("chunk size: %d\n", len(getPNG()))
	fmt.Println("Content-Type: image/png")
	for i:=0; i<2; i++ {
		binary.Write(os.Stdout, binary.LittleEndian, getPNG())
	}
}

func getPNG() []byte {
	str := strings.ReplaceAll(png, "\n", "")
	str = strings.ReplaceAll(str, " ", "")
	x, err := hex.DecodeString(str)
	if err != nil {
		panic(err.Error())
	}
	return x
}
const (
	png = ` + "`" + `
89504e470d0a1a0a0000000d494844520000001000000010080300000028
2d0f53000000a2504c544576e1feffffff0920257aeaff0a2126000000b4
b7b87f7d7c72e3ff78e7fff8fafac0c5c6f5f6f7c6cbcd18313853616598
9ea00020262a56614380914c95a832494f62b8cf559aad30758698e2f6a8
f3ff80e8ff84ddf53b778758a4b9293d4255686d68c6e0e4ffff65c1da4f
5a5eb9ebfad5f6ffd1d0d0adbdc06ad5f239474b13282db8f3ffd9feff23
3337002f37a3eeffc58261996751bf876ce9d6cfafd1da07e881e1000000
92494441541895adcdd90e82400c05d03b28da11650765914514774584ff
ff353bc4187df73eb437274d0ac19919138e31571d0bd3b21dd7f37dcf75
6ccb341084cb55144b49499caeb32c4751961b7599f288ab6a0b11709dea
bb5aecf5c3f12420ce8982cb5501dd18b451cdd7f79c4723b501286a1e6d
fb6c00391e00a0b0eb42fa0250df935a1f00117ee19d3f00bf7d0139970a
1c750341ac0000000049454e44ae426082
` + "`" + `
)
`
