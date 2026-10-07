package utils

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

var dedentMarginChar = regexp.MustCompile(`^[ \t]*`)

type dedentOpt struct {
	replaceTabBySpaces int
}

func (d *dedentOpt) fillDefault() *dedentOpt {
	d.replaceTabBySpaces = 4
	return d
}

func (d *dedentOpt) applyOpts(optfs ...DedentOptFunc) *dedentOpt {
	for _, optf := range optfs {
		optf(d)
	}
	return d
}

// DedentOptFunc dedent option
type DedentOptFunc func(opt *dedentOpt)

// WithReplaceTabBySpaces selects 0..256 spaces per leading tab.
// Values outside this range use the default width of four spaces.
func WithReplaceTabBySpaces(spaces int) DedentOptFunc {
	// Keep invalid options from causing a panic or unbounded tab expansion.
	if spaces < 0 || spaces > 256 {
		spaces = 4
	}
	return func(opt *dedentOpt) {
		opt.replaceTabBySpaces = spaces
	}
}

// Dedent removes leading whitespace or tab from the beginning of each line
//
// will replace all tab to 4 blanks.
func Dedent(v string, optfs ...DedentOptFunc) string {
	opt := new(dedentOpt).fillDefault().applyOpts(optfs...)
	ls := strings.Split(v, "\n")
	var (
		NSpaceTobeTrim int
		firstLine      = true
		result         = make([]string, 0, len(ls))
	)
	for _, l := range ls {
		if strings.TrimSpace(l) == "" {
			if !firstLine {
				result = append(result, "")
			}

			continue
		}

		m := dedentMarginChar.FindString(l)
		spaceIndent := strings.ReplaceAll(m, "\t", strings.Repeat(" ", opt.replaceTabBySpaces))
		n := len(spaceIndent)
		l = strings.Replace(l, m, spaceIndent, 1)
		if firstLine {
			NSpaceTobeTrim = n
			firstLine = false
		} else if n < NSpaceTobeTrim {
			// choose the smallest margin
			NSpaceTobeTrim = n
		}

		result = append(result, l)
	}

	for i := range result {
		if result[i] == "" {
			continue
		}

		result[i] = result[i][NSpaceTobeTrim:]
	}

	// remove tail blank lines
	for i := len(result) - 1; i >= 0; i-- {
		if result[i] == "" {
			result = result[:i]
		} else {
			break
		}
	}

	return strings.Join(result, "\n")
}

// RegexNamedSubMatch extract key:val map from string by group match
//
// Deprecated: use RegexNamedSubMatch2 instead
func RegexNamedSubMatch(r *regexp.Regexp, str string, subMatchMap map[string]string) error {
	match := r.FindStringSubmatch(str)
	names := r.SubexpNames()
	if len(names) != len(match) {
		return errors.New("the number of args in `regexp` and `str` not matched")
	}

	for i, name := range r.SubexpNames() {
		if i != 0 && name != "" {
			subMatchMap[name] = match[i]
		}
	}

	return nil
}

// RegexNamedSubMatch2 extract key:val map from string by group match
func RegexNamedSubMatch2(r *regexp.Regexp, str string) (subMatchMap map[string]string, err error) {
	match := r.FindStringSubmatch(str)
	names := r.SubexpNames()
	if len(names) != len(match) {
		return nil, errors.New("the number of args in `regexp` and `str` not matched")
	}

	subMatchMap = make(map[string]string)
	for i, name := range r.SubexpNames() {
		if i != 0 && name != "" {
			subMatchMap[name] = match[i]
		}
	}

	return subMatchMap, nil
}

var defaultTemplateWithMappReg = regexp.MustCompile(`(?sm)\$\{([^}]+)\}`)

// TemplateWithMap replace `${var}` in template string
func TemplateWithMap(tpl string, data map[string]any) string {
	return TemplateWithMapAndRegexp(defaultTemplateWithMappReg, tpl, data)
}

// TemplateWithMapAndRegexp replace `${var}` in template string
func TemplateWithMapAndRegexp(tplReg *regexp.Regexp, tpl string, data map[string]any) string {
	var (
		k, vs string
		vi    any
	)
	for _, kg := range tplReg.FindAllStringSubmatch(tpl, -1) {
		k = kg[1]
		vi = data[k]
		switch vi := vi.(type) {
		case string:
			vs = vi
		case []byte:
			vs = string(vi)
		case int:
			vs = strconv.FormatInt(int64(vi), 10)
		case int64:
			vs = strconv.FormatInt(vi, 10)
		case float64:
			vs = strconv.FormatFloat(vi, 'f', -1, 64)
		}
		tpl = strings.ReplaceAll(tpl, "${"+k+"}", vs)
	}

	return tpl
}
