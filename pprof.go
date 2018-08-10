package utils

import (
	"os"
	"runtime/pprof"
	"runtime/trace"
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

func tracePprof() {
	f, err := os.Create("trace.pprod")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	err = trace.Start(f)
	if err != nil {
		panic(err)
	}
	defer trace.Stop()
	time.Sleep(time.Second * 300)
}
