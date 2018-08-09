package utils

import (
	"os"
	"runtime/pprof"
	"time"
)

func memPprof() {
	f, err := os.Create("memory.pprof")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	defer pprof.WriteHeapProfile(f)

	time.Sleep(time.Second * 300)
}

func cpuPprof() {
	f, err := os.Create("cpu.pprof")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()
	time.Sleep(time.Second * 300)
}
