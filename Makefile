test:
	go test -v -count=1 ./...

race:
	go test -v -race -count=1 ./...

coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic; \
	go tool cover -html=coverage.out