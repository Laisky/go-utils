init:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.27.0
	go get golang.org/x/tools/cmd/goimports
	go get -u github.com/golang/protobuf/protoc-gen-go@v1.3.2

lint:
	# goimports -local github.com/Laisky -w .
	gofmt -s -w .
	go mod tidy
	golangci-lint run -E depguard,gocognit,goconst,gofmt,misspell,golint,gomnd

changelog:
	./.scripts/generate_changelog.sh
