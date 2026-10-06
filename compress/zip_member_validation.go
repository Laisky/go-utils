package compress

import (
	"path"
	"strings"

	"github.com/Laisky/errors/v2"
)

// validateZIPMemberName requires canonical, portable, relative ZIP entry names.
// Directory entries may have one trailing slash. Parent/current components,
// Windows volumes/streams, backslashes, absolute names, and empty components are
// rejected before extraction, independently of the host operating system.
func validateZIPMemberName(name string) error {
	if name == "" || path.IsAbs(name) || strings.ContainsAny(name, "\\:\x00") {
		return errors.New("ZIP member must be a portable relative path")
	}
	name = strings.TrimSuffix(name, "/")
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." || component == ".." {
			return errors.New("ZIP member contains a noncanonical path component")
		}
	}
	return nil
}
