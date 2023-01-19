.PHONY: install
install:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	# go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.1
	# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2.0
	# go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway
	# go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2
	go install github.com/vektra/mockery/v2@latest

lint:
	# goimports -local github.com/Laisky -w .
	go vet
	gofmt -s -w .
	go mod tidy
	golangci-lint run -c .golangci.lint.yml

changelog:
	./.scripts/generate_changelog.sh
