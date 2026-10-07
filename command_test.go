package utils

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunCMD(t *testing.T) {
	ctx := context.Background()
	type args struct {
		app  string
		args []string
	}
	tests := []struct {
		name       string
		args       args
		wantStdout []byte
		wantErr    bool
	}{
		{"sleep", args{"sleep", []string{"0.1"}}, []byte{}, false},
		{"sleep-err", args{"sleep", nil}, []byte("sleep: missing operand"), true},
		{"reject-cmd-substitution-in-app", args{"$(echo sleep)", nil}, nil, true},
		{"reject-newline-in-app", args{"sleep\necho", nil}, nil, true},
		{"reject-null-in-app", args{"sleep\x00evil", nil}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStdout, err := RunCMD(ctx, tt.args.app, tt.args.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunCMD() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !bytes.Contains(gotStdout, tt.wantStdout) {
				t.Errorf("RunCMD() = %s, want %s", gotStdout, tt.wantStdout)
			}
		})
	}
}

// linux pipe has 16MB default buffer
func TestRunCMDForHugeFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "run_cmd-*")
	require.NoError(t, err)
	defer os.Remove(dir)

	fpath := filepath.Join(dir, "test.txt")
	fp, err := os.OpenFile(fpath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0664)
	require.NoError(t, err)

	for i := 0; i < 1024*18; i++ {
		_, err = fp.Write([]byte(RandomStringWithLength(1024)))
		require.NoError(t, err)
	}
	err = fp.Close()
	require.NoError(t, err)

	ctx := context.Background()
	out, err := RunCMDWithOptions(ctx, "cat", []string{fpath}, nil, CMDOptions{MaxOutputBytes: 18 * 1024 * 1024})
	require.NoError(t, err)
	require.Equal(t, len(out), 18*1024*1024)
}

func TestRunCMDWithEnv(t *testing.T) {
	ctx := context.Background()

	type args struct {
		ctx  context.Context
		app  string
		args []string
		envs []string
	}
	tests := []struct {
		name       string
		args       args
		wantStdout []byte
		wantErr    bool
	}{
		// {"", args{ctx, `/bin/env`, nil, []string{"FOO=BAR"}}, []byte("BAR"), false},
		{"", args{ctx, `/bin/bash`, []string{"-c", "echo $FOO"}, []string{"FOO=BAR"}}, []byte("BAR\n"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStdout, err := RunCMDWithEnv(tt.args.ctx, tt.args.app, tt.args.args, tt.args.envs)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunCMDWithEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotStdout, tt.wantStdout) {
				t.Errorf("RunCMDWithEnv() = %q, want %q", string(gotStdout), string(tt.wantStdout))
			}
		})
	}
}

func TestRunCMD2(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "TestRunCMD2-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// write shell file
	execFile := filepath.Join(dir, "test.sh")
	err = os.WriteFile(execFile, []byte(Dedent(
		`#!/bin/bash

		while true; do
			echo "hello"
			sleep 0.1
		done`)), 0755)
	require.NoError(t, err)

	// run shell file
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var (
		stdoutMu sync.Mutex
		stdout   []string
	)
	stdoutHandler := func(msg string) {
		stdoutMu.Lock()
		defer stdoutMu.Unlock()
		stdout = append(stdout, msg)
	}
	go RunCMD2(ctx, "/bin/bash", []string{execFile}, nil, stdoutHandler, nil)
	time.Sleep(time.Second)
	cancel()

	stdoutMu.Lock()
	require.Greater(t, len(stdout), 5)
	require.Contains(t, stdout[0], "hello")
	stdoutMu.Unlock()
}
