BIN := bin/goinv
RUN := go run ./cmd/goinv
DAYS ?= 30

.PHONY: run build test vet fmt logout status snapshot history clean

run:
	@$(RUN)

build:
	go build -o $(BIN) ./cmd/goinv

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

logout:
	@$(RUN) logout

status:
	@$(RUN) status

# Non-interactive, meant for cron.
snapshot:
	@$(RUN) snapshot

history:
	@$(RUN) history $(DAYS)

clean:
	rm -rf bin
