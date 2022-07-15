init:
	go get golang.org/x/tools/cmd/goimports
	go get -u github.com/golang/protobuf/protoc-gen-go@v1.3.2
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.46.2

lint:
	# goimports -local github.com/Laisky -w .
	go vet
	gofmt -s -w .
	go mod tidy
	golangci-lint run -c .golangci.lint.yml

changelog:
	./.scripts/generate_changelog.sh
