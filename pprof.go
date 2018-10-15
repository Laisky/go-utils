package utils

import (
	"os"
	"runtime/pprof"
	"runtime/trace"
)

func EnableMemPprof() {
	f, err := os.Create("memory.pprof")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	defer pprof.WriteHeapProfile(f)
}

func EnableCpuPprof() {
	f, err := os.Create("cpu.pprof")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()
}

func EnableTracePprof() {
	f, err := os.Create("trace.pprof")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	err = trace.Start(f)
	if err != nil {
		panic(err)
	}
	defer trace.Stop()
}
