package utils

import (
	"runtime/debug"

	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/go-utils/v6/log"
)

type prettyBuildInfoOption struct {
	withDeps bool
}

func (o *prettyBuildInfoOption) apply(fs ...PrettyBuildInfoOption) *prettyBuildInfoOption {
	for _, f := range fs {
		f(o)
	}

	return o
}

// PrettyBuildInfoOption options for PrettyBuildInfo
type PrettyBuildInfoOption func(*prettyBuildInfoOption)

// WithPrettyBuildInfoDeps include deps in build info
func WithPrettyBuildInfoDeps() PrettyBuildInfoOption {
	return func(opt *prettyBuildInfoOption) {
		opt.withDeps = true
	}
}

// PrettyBuildInfo get build info in formatted json
//
// Print:
//
//	{
//	  "Path": "github.com/Laisky/go-ramjet",
//	  "Version": "v0.0.0-20220718014224-2b10e57735f1",
//	  "Sum": "h1:08Ty2gR+Xxz0B3djHVuV71boW4lpNdQ9hFn4ZIGrhec=",
//	  "Replace": null
//	}
func PrettyBuildInfo(opts ...PrettyBuildInfoOption) string {
	opt := new(prettyBuildInfoOption).apply(opts...)

	info, ok := debug.ReadBuildInfo()
	if !ok {
		log.Shared.Error("failed to read build info")
		return ""
	}

	type prettyBuildInfoPayload struct {
		GoVersion string               `json:"GoVersion"`
		Path      string               `json:"Path"`
		Main      debug.Module         `json:"Main"`
		Deps      []*debug.Module      `json:"Deps"`
		Settings  []debug.BuildSetting `json:"Settings"`
	}

	payload := prettyBuildInfoPayload{
		GoVersion: info.GoVersion,
		Path:      info.Path,
		Main:      info.Main,
		Settings:  info.Settings,
	}

	if opt.withDeps {
		payload.Deps = info.Deps
	}

	ver, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Shared.Error("failed to marshal version", zap.Error(err))
		return ""
	}

	return string(ver)
}
