init:
	go get golang.org/x/tools/cmd/goimports

lint:
	# goimports -local github.com/Laisky -w .
	gofmt -s -w .
	go mod tidy
	golangci-lint run
