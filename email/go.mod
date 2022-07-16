module github.com/Laisky/go-utils/v2/email

go 1.18

require (
	github.com/Laisky/go-utils/v2 v2.1.0
	github.com/Laisky/go-utils/v2/log v0.0.1
	github.com/Laisky/zap v1.19.3-0.20220707055623-fe1750cd1b41
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.8.0
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
)

require (
	github.com/Laisky/graphql v1.0.5 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/objx v0.4.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/lint v0.0.0-20201208152925-83fdc39ff7b5 // indirect
	golang.org/x/tools v0.1.10 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/Laisky/go-utils/v2 v2.1.0 => ../.
	github.com/Laisky/go-utils/v2/log v0.0.1 => ../log
)
