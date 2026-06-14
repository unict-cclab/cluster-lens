.PHONY: run test

run:
	cd backend && go run .

test:
	cd backend && go test ./...

