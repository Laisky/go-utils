package utils

import (
	"bytes"
	"html/template"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

var (
	Logger log.LoggerInterface
)

// SetupLogger 初始化日志
func SetupLogger(defaultLevel string) {
	var (
		err  error
		args = struct {
			LogLevel string
		}{
			LogLevel: defaultLevel,
		}
	)
	logConfig := `
		<seelog type="asynctimer" asyncinterval="1000000" minlevel="{{.LogLevel}}" maxlevel="error">
			<exceptions>
				<exception funcpattern="*main.test*Something*" minlevel="{{.LogLevel}}"/>
				<exception filepattern="*main.go" minlevel="{{.LogLevel}}"/>
			</exceptions>
			<outputs formatid="main">
				<console/>
			</outputs>
			<formats>
				<format id="main" format="[%UTCDate(2006-01-02T15:04:05.000000Z) - %LEVEL - %RelFile:%Line] %Msg%n"/>
			</formats>
		</seelog>
	`
	tmpl, err := template.New("seelogConfig").Parse(logConfig)
	if err != nil {
		panic(errors.Wrap(err, "parse log config error"))
	}
	var configBytes bytes.Buffer
	if err := tmpl.Execute(&configBytes, args); err != nil {
		panic(errors.Wrap(err, "execute log template error"))
	}
	Logger, err = log.LoggerFromConfigAsBytes(configBytes.Bytes())
	if err != nil {
		panic(errors.Wrap(err, "setup logger by template error"))
	}
	Logger.Info("SetupLogger ok")
	log.ReplaceLogger(Logger)
}

func init() {
	SetupLogger("info")
}
